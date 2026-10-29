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
	"sync"
	"time"

	"github.com/padalton/office-traffic-light/hub/internal/calendar"
	"github.com/padalton/office-traffic-light/hub/internal/engine"
	"github.com/padalton/office-traffic-light/hub/internal/status"
	"github.com/padalton/office-traffic-light/hub/internal/store"
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
	keyFocus    = "focus"
)

// Service is safe for concurrent use.
type Service struct {
	base  engine.Inputs // location, rules, work week, holidays
	store *store.Store
	cal   calendar.Provider
	now   func() time.Time
	log   *slog.Logger

	mu        sync.Mutex
	override  *engine.Override
	focus     *engine.Focus
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
	Base     engine.Inputs
	Store    *store.Store
	Calendar calendar.Provider // nil: no calendar
	Now      func() time.Time  // nil: wall clock in Base.Location
	Logger   *slog.Logger
}

// New creates a service and restores the persisted override/focus timer.
func New(o Options) (*Service, error) {
	s := &Service{
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
	var ov engine.Override
	switch err := s.store.Get(keyOverride, &ov); {
	case err == nil:
		s.override = &ov
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	var f engine.Focus
	switch err := s.store.Get(keyFocus, &f); {
	case err == nil:
		s.focus = &f
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	return s, nil
}

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

// RefreshCalendar fetches events for the planning horizon. On error the
// previous events are kept.
func (s *Service) RefreshCalendar(ctx context.Context) {
	if s.cal == nil {
		return
	}
	now := s.now()
	ev, err := s.cal.Events(ctx, now.Add(-s.base.Rules.Lookback), now.Add(s.base.Rules.Horizon))
	if err != nil {
		s.log.Warn("calendar refresh failed, keeping previous events", "err", err)
		return
	}
	s.mu.Lock()
	s.events = ev
	s.mu.Unlock()
}

func (s *Service) inputsLocked() engine.Inputs {
	in := s.base
	in.Override, in.Focus, in.Companion = s.override, s.focus, s.companion
	in.Events = s.events
	in.Signals = slices.Collect(maps.Values(s.signals))
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
		if err := s.store.AppendHistory(store.HistoryEntry{At: now, Status: string(st.Status), Reason: st.Reason, Detail: st.Detail}); err != nil {
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
	if o := s.override; o != nil && !o.Until.IsZero() && !o.Until.After(now) {
		s.override = nil
		s.persist(keyOverride, nil)
	}
	if f := s.focus; f != nil && !f.End().IsZero() && !f.End().After(now) {
		s.focus = nil
		s.persist(keyFocus, nil)
	}
	for k, sig := range s.signals {
		if !sig.Expires.After(now) {
			delete(s.signals, k)
		}
	}
}

func (s *Service) persist(key string, v any) {
	if err := s.store.Put(key, v); err != nil {
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
)

// OverrideRequest sets a manual status.
type OverrideRequest struct {
	Status   status.Status
	Mode     string
	Duration time.Duration // ModeDuration
	Until    time.Time     // ModeUntil
}

// SetOverride sets a manual status; it replaces a running focus timer.
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
		end, ok := s.base.WorkWeek.EndOfWorkday(now, s.base.Location, s.base.Holidays.IsHoliday)
		if !ok {
			s.mu.Unlock()
			return engine.State{}, fmt.Errorf("%w: outside working hours", ErrConflict)
		}
		ov.Until = end
	case ModeIndefinite:
	default:
		s.mu.Unlock()
		return engine.State{}, fmt.Errorf("%w: mode must be one of duration, until, end_of_event, end_of_workday, indefinite", ErrInvalid)
	}
	s.override, s.focus = &ov, nil
	s.persist(keyOverride, ov)
	s.persist(keyFocus, nil)
	s.mu.Unlock()
	return s.Evaluate(), nil
}

func (s *Service) currentEventEndLocked(now time.Time) (time.Time, bool) {
	var end time.Time
	for _, e := range s.events {
		if _, ok := s.base.Rules.CalendarStatus[e.Kind]; !ok || e.AllDay || e.Transparent {
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

// FocusRequest starts a pomodoro timer.
type FocusRequest struct {
	Work, Break time.Duration
	Cycles      int
	WorkStatus  status.Status
}

// SetFocus starts a focus timer; it replaces a running override.
func (s *Service) SetFocus(r FocusRequest) (engine.State, error) {
	if r.Work < time.Minute || r.Work > 4*time.Hour || r.Break < 0 || r.Break > time.Hour || r.Cycles < 0 || r.Cycles > 24 {
		return engine.State{}, fmt.Errorf("%w: work 1m–4h, break 0–1h, cycles 0–24", ErrInvalid)
	}
	if r.WorkStatus == "" {
		r.WorkStatus = status.Working
	}
	if r.WorkStatus != status.Working && r.WorkStatus != status.DND {
		return engine.State{}, fmt.Errorf("%w: work_status must be working or dnd", ErrInvalid)
	}
	s.mu.Lock()
	f := engine.Focus{Start: s.now(), Work: r.Work, Break: r.Break, Cycles: r.Cycles, WorkStatus: r.WorkStatus}
	s.focus, s.override = &f, nil
	s.persist(keyFocus, f)
	s.persist(keyOverride, nil)
	s.mu.Unlock()
	return s.Evaluate(), nil
}

// ClearFocus stops the focus timer.
func (s *Service) ClearFocus() engine.State {
	s.mu.Lock()
	s.focus = nil
	s.persist(keyFocus, nil)
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
