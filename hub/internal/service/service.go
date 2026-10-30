// Package service holds the live hub state: it collects inputs from the
// API and the calendar, runs the engine, and fans out state changes to
// subscribers (SSE) and publishers (MQTT).
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/padalton/doormutex/hub/internal/calendar"
	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/status"
	"github.com/padalton/doormutex/hub/internal/store"
)

// Errors returned for invalid requests.
var (
	ErrInvalid  = errors.New("invalid request")
	ErrConflict = errors.New("conflict")
)

// Change describes a status transition.
type Change struct {
	From   status.Status `json:"from"`
	To     status.Status `json:"to"`
	Reason string        `json:"reason"`
	Detail string        `json:"detail,omitempty"`
	At     time.Time     `json:"at"`
}

// Publisher receives every new state (e.g. MQTT).
type Publisher interface {
	PublishState(engine.State)
	PublishChange(Change)
}

const (
	keyOverride = "override"
	keyPomodoro = "pomodoro"
	keyMessage  = "message"
	keySettings = "settings"
)

// Ignore is the calendar setting for event kinds the hub skips.
const Ignore = "ignore"

// Settings are the rules people change at runtime (web UI). They are
// persisted and override the config file.
type Settings struct {
	Calendar   map[calendar.Kind]string `json:"calendar"`  // status or "ignore"
	Tentative  string                   `json:"tentative"` // status, "kind" or "ignore"
	LastCall   engine.Lead              `json:"last_call"`
	EndingSoon engine.Lead              `json:"ending_soon"`
	Pomodoro   status.Status            `json:"pomodoro_work_status"` // dnd or working
}

// Message is a short text for the e-ink, optionally with an expiry.
type Message struct {
	Text  string    `json:"text"`
	Until time.Time `json:"until,omitzero"`
}

// MaxMessage is the longest message in characters (the 1.54" e-ink shows
// about four lines of 16 characters).
const MaxMessage = 64

// Service holds one profile's live state. It is safe for concurrent use.
type Service struct {
	id    string
	base  engine.Inputs // location, rules, work week, holidays
	store *store.Store
	cal   calendar.Provider
	now   func() time.Time
	log   *slog.Logger

	mu        sync.Mutex
	override  *engine.Override
	pomodoro  *engine.Pomodoro
	message   *Message
	companion engine.Companion
	signals   map[string]engine.Signal
	events    []calendar.Event
	state     engine.State
	hasState  bool
	subs      map[chan engine.State]struct{}
	pubs      []Publisher
}

// Options configure a Service.
type Options struct {
	Profile  string // profile id; prefixes the persisted keys and the history
	Base     engine.Inputs
	Store    *store.Store
	Calendar calendar.Provider // nil: no calendar
	Now      func() time.Time  // nil: wall clock in Base.Location
	Logger   *slog.Logger
}

// New creates a service and restores the persisted override/pomodoro timer.
func New(o Options) (*Service, error) {
	s := &Service{
		id:      o.Profile,
		base:    o.Base,
		store:   o.Store,
		cal:     o.Calendar,
		now:     o.Now,
		log:     o.Logger,
		signals: map[string]engine.Signal{},
		subs:    map[chan engine.State]struct{}{},
	}
	if s.now == nil {
		// Whole seconds in the configured zone, so every timestamp in the
		// state reads like "2026-10-28T14:30:00+01:00".
		loc := o.Base.Location
		if loc == nil {
			loc = time.Local
		}
		s.now = func() time.Time { return time.Now().In(loc).Truncate(time.Second) }
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	var err error
	if s.override, err = restore[engine.Override](s, keyOverride); err != nil {
		return nil, err
	}
	if s.pomodoro, err = restore[engine.Pomodoro](s, keyPomodoro); err != nil {
		return nil, err
	}
	if s.message, err = restore[Message](s, keyMessage); err != nil {
		return nil, err
	}
	saved, err := restore[Settings](s, keySettings)
	if err != nil {
		return nil, err
	}
	if saved != nil {
		s.applyLocked(*saved)
	}
	return s, nil
}

func restore[T any](s *Service, key string) (*T, error) {
	var v T
	switch err := s.store.Get(s.key(key), &v); {
	case err == nil:
		return &v, nil
	case errors.Is(err, store.ErrNotFound):
		return nil, nil
	default:
		return nil, err
	}
}

// key namespaces a persisted key by profile.
func (s *Service) key(k string) string {
	if s.id == "" {
		return k
	}
	return "profiles/" + s.id + "/" + k
}

// Profile returns the profile id.
func (s *Service) Profile() string { return s.id }

// AddPublisher registers a publisher; it immediately gets the current state.
func (s *Service) AddPublisher(p Publisher) {
	s.mu.Lock()
	s.pubs = append(s.pubs, p)
	st, ok := s.state, s.hasState
	s.mu.Unlock()
	if ok {
		p.PublishState(st)
	}
}

// Run evaluates every tick and refreshes the calendar every poll interval
// until ctx is done.
func (s *Service) Run(ctx context.Context, tick, poll time.Duration) {
	s.RefreshCalendar(ctx)
	s.Evaluate()
	t := time.NewTicker(tick)
	defer t.Stop()
	p := time.NewTicker(poll)
	defer p.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Evaluate()
		case <-p.C:
			s.RefreshCalendar(ctx)
			s.Evaluate()
		}
	}
}

// CalendarStatus reports the calendar accounts (nil without a calendar or
// for providers without accounts).
func (s *Service) CalendarStatus() []calendar.AccountStatus {
	if r, ok := s.cal.(calendar.StatusReporter); ok {
		return r.Status()
	}
	return nil
}

// RefreshCalendar fetches events for the planning horizon. On error the
// previous events are kept.
func (s *Service) RefreshCalendar(ctx context.Context) {
	if s.cal == nil {
		return
	}
	now := s.now()
	ev, err := s.cal.Events(ctx, now.Add(-s.base.Rules.Lookback), now.Add(s.base.Rules.Horizon))
	if err != nil {
		if ev == nil {
			s.log.Warn("calendar refresh failed, keeping previous events", "err", err)
			return
		}
		s.log.Warn("calendar refresh partly failed", "err", err)
	}
	s.mu.Lock()
	s.events = ev
	s.mu.Unlock()
}

func (s *Service) inputsLocked() engine.Inputs {
	in := s.base
	in.Override, in.Pomodoro, in.Companion = s.override, s.pomodoro, s.companion
	in.Events = s.events
	in.Signals = slices.Collect(maps.Values(s.signals))
	if s.message != nil {
		in.Message = s.message.Text
	}
	return in
}

// Evaluate recomputes the state and notifies everyone if it changed.
func (s *Service) Evaluate() engine.State {
	s.mu.Lock()
	now := s.now()
	s.expireLocked(now)
	st := engine.Evaluate(now, s.inputsLocked())
	prev, had := s.state, s.hasState
	changed := !had || !prev.SameAs(st)
	if !changed {
		s.mu.Unlock()
		return prev
	}
	s.state, s.hasState = st, true
	for ch := range s.subs {
		select {
		case ch <- st:
		default: // slow subscriber: drop, it gets the next one
		}
	}
	pubs := slices.Clone(s.pubs)
	s.mu.Unlock()

	for _, p := range pubs {
		p.PublishState(st)
	}
	if !had || prev.Status != st.Status {
		c := Change{From: prev.Status, To: st.Status, Reason: st.Reason, Detail: st.Detail, At: now}
		s.log.Info("status changed", "from", c.From, "to", c.To, "reason", c.Reason, "detail", c.Detail)
		if err := s.store.AppendHistory(store.HistoryEntry{Profile: s.id, At: now, Status: string(st.Status), Reason: st.Reason, Detail: st.Detail}); err != nil {
			s.log.Error("write history", "err", err)
		}
		if had {
			for _, p := range pubs {
				p.PublishChange(c)
			}
		}
	}
	return st
}

func (s *Service) expireLocked(now time.Time) {
	if o := s.override; o != nil && (!o.Until.IsZero() && !o.Until.After(now) || o.Ended(now, s.inputsLocked())) {
		s.override = nil
		s.persist(keyOverride, nil)
	}
	if f := s.pomodoro; f != nil && !f.End().IsZero() && !f.End().After(now) {
		s.pomodoro = nil
		s.persist(keyPomodoro, nil)
	}
	for k, sig := range s.signals {
		if !sig.Expires.After(now) {
			delete(s.signals, k)
		}
	}
	if m := s.message; m != nil && !m.Until.IsZero() && !m.Until.After(now) {
		s.message = nil
		s.persist(keyMessage, nil)
	}
}

func (s *Service) persist(key string, v any) {
	if err := s.store.Put(s.key(key), v); err != nil {
		s.log.Error("persist", "key", key, "err", err)
	}
}

// State returns the current state (evaluating once if needed).
func (s *Service) State() engine.State {
	s.mu.Lock()
	st, ok := s.state, s.hasState
	s.mu.Unlock()
	if !ok {
		return s.Evaluate()
	}
	return st
}

// Timeline returns the planned slots in [from, to).
func (s *Service) Timeline(from, to time.Time) []engine.Slot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return engine.Timeline(s.now(), from, to, s.inputsLocked())
}

// Subscribe returns a channel with every new state, starting with the
// current one. Call the returned function to unsubscribe.
func (s *Service) Subscribe() (<-chan engine.State, func()) {
	st := s.State()
	ch := make(chan engine.State, 8)
	ch <- st
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

// Override modes.
const (
	ModeDuration     = "duration"
	ModeUntil        = "until"
	ModeEndOfEvent   = "end_of_event"
	ModeEndOfWorkday = "end_of_workday"
	ModeIndefinite   = "indefinite"
	ModeNextChange   = "next_change" // until the automatic status changes
)

// RemoteOverrideMode is the mode for overrides from Home Assistant's
// select: with a required desktop companion the next automatic change
// ends it; without one (HA-only, room mode) HA or the web UI is the
// controller and it stays until changed.
func (s *Service) RemoteOverrideMode() string {
	if s.base.Rules.RequireCompanion {
		return ModeNextChange
	}
	return ModeIndefinite
}

// OverrideRequest sets a manual status.
type OverrideRequest struct {
	Status   status.Status
	Mode     string
	Duration time.Duration // ModeDuration
	Until    time.Time     // ModeUntil
}

// SetOverride sets a manual status; it replaces a running pomodoro timer.
func (s *Service) SetOverride(r OverrideRequest) (engine.State, error) {
	if _, err := status.Parse(string(r.Status)); err != nil {
		return engine.State{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	s.mu.Lock()
	now := s.now()
	ov := engine.Override{Status: r.Status, Start: now}
	switch r.Mode {
	case ModeDuration:
		if r.Duration <= 0 || r.Duration > 7*24*time.Hour {
			s.mu.Unlock()
			return engine.State{}, fmt.Errorf("%w: duration must be between 1s and 7 days", ErrInvalid)
		}
		ov.Until = now.Add(r.Duration)
	case ModeUntil:
		if !r.Until.After(now) {
			s.mu.Unlock()
			return engine.State{}, fmt.Errorf("%w: until must be in the future", ErrInvalid)
		}
		ov.Until = r.Until
	case ModeEndOfEvent:
		end, ok := s.currentEventEndLocked(now)
		if !ok {
			s.mu.Unlock()
			return engine.State{}, fmt.Errorf("%w: no calendar event is running", ErrConflict)
		}
		ov.Until = end
	case ModeEndOfWorkday:
		end, ok := s.base.WorkWeek.EndOfWorkday(now, s.base.Location, s.inputsLocked().DayOff)
		if !ok {
			s.mu.Unlock()
			return engine.State{}, fmt.Errorf("%w: outside working hours", ErrConflict)
		}
		ov.Until = end
	case ModeIndefinite:
	case ModeNextChange:
		in := s.inputsLocked()
		in.Pomodoro = nil // replaced by the override
		ov.UntilChange, ov.Base = true, engine.Automatic(now, in)
	default:
		s.mu.Unlock()
		return engine.State{}, fmt.Errorf("%w: mode must be one of duration, until, end_of_event, end_of_workday, next_change, indefinite", ErrInvalid)
	}
	s.override, s.pomodoro = &ov, nil
	s.persist(keyOverride, ov)
	s.persist(keyPomodoro, nil)
	s.mu.Unlock()
	return s.Evaluate(), nil
}

func (s *Service) currentEventEndLocked(now time.Time) (time.Time, bool) {
	var end time.Time
	for _, e := range s.events {
		if _, ok := s.base.Rules.EventStatus(e); !ok {
			continue
		}
		if !now.Before(e.Start) && now.Before(e.End) && e.End.After(end) {
			end = e.End
		}
	}
	return end, !end.IsZero()
}

// ClearOverride returns to automatic mode.
func (s *Service) ClearOverride() engine.State {
	s.mu.Lock()
	s.override = nil
	s.persist(keyOverride, nil)
	s.mu.Unlock()
	return s.Evaluate()
}

// PomodoroRequest starts a pomodoro timer.
type PomodoroRequest struct {
	Work, Break time.Duration
	Cycles      int
	WorkStatus  status.Status
}

// SetPomodoro starts a pomodoro timer; it replaces a running override.
func (s *Service) SetPomodoro(r PomodoroRequest) (engine.State, error) {
	if r.Work < time.Minute || r.Work > 4*time.Hour || r.Break < 0 || r.Break > time.Hour || r.Cycles < 0 || r.Cycles > 24 {
		return engine.State{}, fmt.Errorf("%w: work 1m–4h, break 0–1h, cycles 0–24", ErrInvalid)
	}
	s.mu.Lock()
	if r.WorkStatus == "" {
		r.WorkStatus = s.base.Rules.PomodoroWork
	}
	if r.WorkStatus == "" {
		r.WorkStatus = status.DND
	}
	if err := CheckPomodoroWork(r.WorkStatus); err != nil {
		s.mu.Unlock()
		return engine.State{}, err
	}
	f := engine.Pomodoro{Start: s.now(), Work: r.Work, Break: r.Break, Cycles: r.Cycles, WorkStatus: r.WorkStatus}
	s.pomodoro, s.override = &f, nil
	s.persist(keyPomodoro, f)
	s.persist(keyOverride, nil)
	s.mu.Unlock()
	return s.Evaluate(), nil
}

// ClearPomodoro stops the pomodoro timer.
func (s *Service) ClearPomodoro() engine.State {
	s.mu.Lock()
	s.pomodoro = nil
	s.persist(keyPomodoro, nil)
	s.mu.Unlock()
	return s.Evaluate()
}

// CompanionReport applies a heartbeat from the desktop companion.
func (s *Service) CompanionReport(r engine.Report) engine.State {
	s.mu.Lock()
	s.companion.Observe(s.now(), r, s.base.Rules)
	s.mu.Unlock()
	return s.Evaluate()
}

// Settings returns the current runtime settings.
func (s *Service) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settingsLocked()
}

func (s *Service) settingsLocked() Settings {
	r := s.base.Rules
	out := Settings{Calendar: map[calendar.Kind]string{}, Tentative: r.Tentative, LastCall: r.LastCall, EndingSoon: r.EndingSoon}
	if out.Tentative == "" {
		out.Tentative = engine.TentativeKind
	}
	out.Pomodoro = r.PomodoroWork
	if out.Pomodoro == "" {
		out.Pomodoro = status.DND
	}
	for _, k := range calendar.Kinds {
		out.Calendar[k] = Ignore
		if st, ok := r.CalendarStatus[k]; ok {
			out.Calendar[k] = string(st)
		}
	}
	return out
}

// SettingsUpdate changes some settings; nil fields stay as they are.
type SettingsUpdate struct {
	Calendar   map[calendar.Kind]string
	Tentative  *string
	LastCall   *engine.Lead
	EndingSoon *engine.Lead
	Pomodoro   *status.Status
}

// CheckPomodoroWork validates the status of pomodoro work phases.
func CheckPomodoroWork(s status.Status) error {
	if s != status.Working && s != status.DND {
		return fmt.Errorf("%w: pomodoro work status must be working or dnd", ErrInvalid)
	}
	return nil
}

// UpdateSettings validates, applies and persists a settings change.
func (s *Service) UpdateSettings(u SettingsUpdate) (Settings, error) {
	for k, v := range u.Calendar {
		if !slices.Contains(calendar.Kinds, k) {
			return Settings{}, fmt.Errorf("%w: unknown event kind %q", ErrInvalid, k)
		}
		if v != Ignore {
			if _, err := status.Parse(v); err != nil {
				return Settings{}, fmt.Errorf("%w: calendar.%s: %w", ErrInvalid, k, err)
			}
		}
	}
	if u.Tentative != nil {
		if err := engine.CheckTentative(*u.Tentative); err != nil {
			return Settings{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	if u.Pomodoro != nil {
		if err := CheckPomodoroWork(*u.Pomodoro); err != nil {
			return Settings{}, err
		}
	}
	s.mu.Lock()
	next := s.settingsLocked()
	maps.Copy(next.Calendar, u.Calendar)
	if u.Tentative != nil {
		next.Tentative = *u.Tentative
	}
	if u.Pomodoro != nil {
		next.Pomodoro = *u.Pomodoro
	}
	if u.LastCall != nil {
		next.LastCall = *u.LastCall
	}
	if u.EndingSoon != nil {
		next.EndingSoon = *u.EndingSoon
	}
	s.applyLocked(next)
	s.persist(keySettings, next)
	out := s.settingsLocked()
	s.mu.Unlock()
	s.Evaluate()
	return out, nil
}

func (s *Service) applyLocked(st Settings) {
	cs := map[calendar.Kind]status.Status{}
	for k, v := range st.Calendar {
		if v != Ignore {
			cs[k] = status.Status(v)
		}
	}
	s.base.Rules.CalendarStatus = cs
	// Settings saved by an older hub lack the newer fields: keep the defaults.
	if st.Tentative != "" {
		s.base.Rules.Tentative = st.Tentative
	}
	if st.Pomodoro != "" {
		s.base.Rules.PomodoroWork = st.Pomodoro
	}
	s.base.Rules.LastCall, s.base.Rules.EndingSoon = st.LastCall, st.EndingSoon
}

// SetMessage sets the e-ink message; an empty text clears it. A zero ttl
// keeps it until it is changed.
func (s *Service) SetMessage(text string, ttl time.Duration) (engine.State, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > MaxMessage || strings.ContainsAny(text, "\r\n") {
		return engine.State{}, fmt.Errorf("%w: message: one line, at most %d characters", ErrInvalid, MaxMessage)
	}
	if ttl < 0 || ttl > 7*24*time.Hour {
		return engine.State{}, fmt.Errorf("%w: ttl must be between 0 and 7 days", ErrInvalid)
	}
	s.mu.Lock()
	if text == "" {
		s.message = nil
		s.persist(keyMessage, nil)
	} else {
		m := Message{Text: text}
		if ttl > 0 {
			m.Until = s.now().Add(ttl)
		}
		s.message = &m
		s.persist(keyMessage, m)
	}
	s.mu.Unlock()
	return s.Evaluate(), nil
}

// History returns the most recent status changes, newest first.
func (s *Service) History(limit int) ([]store.HistoryEntry, error) {
	return s.store.History(s.id, limit)
}

var signalName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ReservedSignal is the source name used by the desktop companion.
const ReservedSignal = "companion"

// SetSignal sets a generic webhook claim with a TTL.
func (s *Service) SetSignal(name string, st status.Status, ttl time.Duration, detail string) (engine.State, error) {
	if !signalName.MatchString(name) || name == ReservedSignal {
		return engine.State{}, fmt.Errorf("%w: source must match %s and must not be %q", ErrInvalid, signalName, ReservedSignal)
	}
	if _, err := status.Parse(string(st)); err != nil {
		return engine.State{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if ttl < time.Second || ttl > 24*time.Hour {
		return engine.State{}, fmt.Errorf("%w: ttl must be between 1s and 24h", ErrInvalid)
	}
	if len(detail) > 64 {
		return engine.State{}, fmt.Errorf("%w: detail is limited to 64 characters", ErrInvalid)
	}
	s.mu.Lock()
	now := s.now()
	s.signals[name] = engine.Signal{Name: name, Status: st, Detail: detail, Start: now, Expires: now.Add(ttl)}
	s.mu.Unlock()
	return s.Evaluate(), nil
}

// ClearSignal removes a webhook claim.
func (s *Service) ClearSignal(name string) engine.State {
	s.mu.Lock()
	delete(s.signals, name)
	s.mu.Unlock()
	return s.Evaluate()
}
