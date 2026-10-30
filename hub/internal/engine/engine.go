// Package engine decides the traffic light state. It is pure: given the
// inputs and a point in time it returns the state, so it can be tested
// without clocks, networks or databases.
//
// Every source contributes claims (status, priority, validity interval).
// For every moment the claim with the best (lowest) priority wins.
package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/padalton/doormutex/hub/internal/calendar"
	"github.com/padalton/doormutex/hub/internal/holidays"
	"github.com/padalton/doormutex/hub/internal/schedule"
	"github.com/padalton/doormutex/hub/internal/status"
)

// Priority of a claim; lower wins. Mirrors the table in the README.
type Priority int

const (
	PrioMedia     Priority = 1 // microphone or camera in use – beats everything
	PrioPresence  Priority = 2 // screen locked, PC off, companion silent
	PrioManual    Priority = 3 // override, pomodoro timer
	PrioCalendar  Priority = 4
	PrioWorkHours Priority = 5
	PrioSignal    Priority = 6 // generic webhook signals
	PrioDefault   Priority = 7
)

// Reason codes reported in the state.
const (
	ReasonOverride         = "override"
	ReasonPomodoro         = "pomodoro"
	ReasonPomodoroBreak    = "pomodoro_break"
	ReasonMicrophone       = "microphone"
	ReasonCamera           = "camera"
	ReasonScreenLocked     = "screen_locked"
	ReasonPCOff            = "pc_off"            // the companion signed off (shutdown, suspend)
	ReasonCompanionLost    = "companion_lost"    // silent without signing off → fault
	ReasonCompanionOffline = "companion_offline" // never seen, or silent outside working hours
	ReasonCalendar         = "calendar"
	ReasonWorkHours        = "work_hours"
	ReasonSignal           = "signal"
	ReasonDefault          = "default"
)

// Claim is one source's opinion for an interval. A zero End means open-ended.
type Claim struct {
	Status   status.Status
	Priority Priority
	Reason   string
	Detail   string
	Start    time.Time
	End      time.Time
	planned  bool // known in advance (calendar, work hours, timers) vs. live
}

// Override is a manual status. A zero Until means indefinite – unless
// UntilChange is set: then it ends as soon as the automatic status (what
// the light would show without the override) differs from Base.
type Override struct {
	Status      status.Status `json:"status"`
	Start       time.Time     `json:"start"`
	Until       time.Time     `json:"until,omitzero"`
	UntilChange bool          `json:"until_change,omitempty"`
	Base        status.Status `json:"base,omitempty"` // automatic status when it was set
}

// Ended reports whether an until-change override is over: the automatic
// status at now differs from the one it was set on.
func (o Override) Ended(now time.Time, in Inputs) bool {
	if !o.UntilChange {
		return false
	}
	return Automatic(now, in) != o.Base
}

// Automatic returns the status at now without the override.
func Automatic(now time.Time, in Inputs) status.Status {
	in.Override = nil
	return Evaluate(now, in).Status
}

// Pomodoro is a timer: Work, then Break, repeated Cycles times
// (0 = until stopped). WorkStatus is shown during work phases.
type Pomodoro struct {
	Start      time.Time     `json:"start"`
	Work       time.Duration `json:"work"`
	Break      time.Duration `json:"break"`
	Cycles     int           `json:"cycles"`
	WorkStatus status.Status `json:"work_status"`
}

type pomodoroJSON struct {
	Start      time.Time     `json:"start"`
	Work       string        `json:"work"`
	Break      string        `json:"break"`
	Cycles     int           `json:"cycles"`
	WorkStatus status.Status `json:"work_status"`
	End        *time.Time    `json:"end"`
}

// MarshalJSON writes durations as "25m0s" and adds the end time.
func (f Pomodoro) MarshalJSON() ([]byte, error) {
	j := pomodoroJSON{Start: f.Start, Work: f.Work.String(), Break: f.Break.String(), Cycles: f.Cycles, WorkStatus: f.WorkStatus}
	if e := f.End(); !e.IsZero() {
		j.End = &e
	}
	return json.Marshal(j)
}

// UnmarshalJSON implements json.Unmarshaler.
func (f *Pomodoro) UnmarshalJSON(b []byte) error {
	var j pomodoroJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	w, err1 := time.ParseDuration(j.Work)
	br, err2 := time.ParseDuration(j.Break)
	if err := errors.Join(err1, err2); err != nil {
		return err
	}
	*f = Pomodoro{Start: j.Start, Work: w, Break: br, Cycles: j.Cycles, WorkStatus: j.WorkStatus}
	return nil
}

// End returns when the pomodoro session ends (zero if unlimited).
func (f Pomodoro) End() time.Time {
	if f.Cycles <= 0 {
		return time.Time{}
	}
	return f.Start.Add(time.Duration(f.Cycles) * (f.Work + f.Break))
}

// Signal is a generic webhook claim with a TTL.
type Signal struct {
	Name    string        `json:"name"`
	Status  status.Status `json:"status"`
	Detail  string        `json:"detail,omitempty"`
	Start   time.Time     `json:"start"`
	Expires time.Time     `json:"expires"`
}

// StVO timings (Austrian road traffic regulations, § 38): green blinks
// four times before yellow, yellow shows about 3 s before red, red+yellow
// about 2 s before green.
const (
	StVOLastCall   = 4 * time.Second
	StVOEndingSoon = 2 * time.Second
	AmberTime      = 3 * time.Second
)

// Lead is how long a warning phase lasts before the change. Zero means
// StVO timing; a positive value turns the phase into a real warning.
type Lead time.Duration

// Or returns the lead time, or stvo if it is zero.
func (l Lead) Or(stvo time.Duration) time.Duration {
	if l <= 0 {
		return stvo
	}
	return time.Duration(l)
}

// MarshalText implements encoding.TextMarshaler: "stvo" or a duration.
func (l Lead) MarshalText() ([]byte, error) {
	if l <= 0 {
		return []byte("stvo"), nil
	}
	return []byte(time.Duration(l).String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (l *Lead) UnmarshalText(b []byte) error {
	if string(b) == "stvo" || len(b) == 0 {
		*l = 0
		return nil
	}
	d, err := time.ParseDuration(string(b))
	if err != nil || d <= 0 || d > time.Hour {
		return fmt.Errorf("lead time %q: want \"stvo\" or a duration between 1s and 1h", b)
	}
	*l = Lead(d)
	return nil
}

// Rules are the tunables of the decision logic.
type Rules struct {
	CalendarStatus   map[calendar.Kind]status.Status // missing kind → event ignored
	Tentative        string                          // tentative events: a status, TentativeKind or TentativeIgnore
	LastCall         Lead                            // warning before a meeting starts
	EndingSoon       Lead                            // red+yellow before a meeting ends
	MediaDebounce    time.Duration                   // mic/cam must be active this long
	MediaHold        time.Duration                   // keep red this long after mic/cam stop
	StaleAfter       time.Duration                   // companion silent this long → away
	RequireCompanion bool                            // no companion heartbeat → away
	Default          status.Status                   // when nothing else applies; empty means free
	PomodoroWork     status.Status                   // pomodoro work phases unless the request says; empty means dnd
	Horizon          time.Duration                   // how far ahead to plan
	Lookback         time.Duration                   // how far back to find "since"
}

// Settings for tentative events (answered "maybe" or not yet) besides a status.
const (
	TentativeKind   = "kind"   // like an accepted event of the same kind
	TentativeIgnore = "ignore" // skip them
)

// CheckTentative validates the setting for tentative events.
func CheckTentative(v string) error {
	if v == TentativeKind || v == TentativeIgnore {
		return nil
	}
	if _, err := status.Parse(v); err != nil {
		return fmt.Errorf("tentative: want a status, %q or %q", TentativeKind, TentativeIgnore)
	}
	return nil
}

// EventStatus returns the status of a timed calendar event; ok is false
// if the rules ignore it. Tentative events get the Tentative status – if
// their kind isn't ignored altogether.
func (r Rules) EventStatus(e calendar.Event) (st status.Status, ok bool) {
	st, ok = r.CalendarStatus[e.Kind]
	if !ok || e.AllDay || e.Transparent || !e.End.After(e.Start) {
		return "", false
	}
	if e.Tentative {
		switch r.Tentative {
		case "", TentativeKind:
		case TentativeIgnore:
			return "", false
		default:
			st = status.Status(r.Tentative)
		}
	}
	return st, true
}

// DefaultRules returns the defaults documented in the README.
func DefaultRules() Rules {
	return Rules{
		CalendarStatus: map[calendar.Kind]status.Status{
			calendar.KindVideoCall:   status.DND,
			calendar.KindFocus:       status.Working,
			calendar.KindOutOfOffice: status.DND, // timed; all-day out of office is a day off
			calendar.KindOther:       status.Working,
		},
		Tentative:        string(status.Working),
		MediaDebounce:    10 * time.Second,
		MediaHold:        2 * time.Minute,
		StaleAfter:       90 * time.Second,
		RequireCompanion: true,
		Horizon:          7 * 24 * time.Hour,
		Lookback:         24 * time.Hour,
	}
}

// Inputs is everything the engine needs at one moment.
type Inputs struct {
	Location  *time.Location
	Rules     Rules
	Override  *Override
	Pomodoro  *Pomodoro
	Companion Companion
	Events    []calendar.Event
	WorkWeek  schedule.Week
	Holidays  *holidays.Calendar
	Signals   []Signal
	Message   string // short text for the e-ink, set by people (web UI, HA)
}

// Slot is a stretch of time with one status. A nil Until means unknown.
type Slot struct {
	Status status.Status `json:"status"`
	Reason string        `json:"reason,omitempty"`
	Since  time.Time     `json:"since"`
	Until  *time.Time    `json:"until"`
}

// FreeSlot is the next stretch of time with status free.
type FreeSlot struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end"`
}

// State is the decision published to every client.
type State struct {
	Status   status.Status `json:"status"`
	Phase    status.Phase  `json:"phase"`
	Light    status.Light  `json:"light"`
	Reason   string        `json:"reason"`
	Detail   string        `json:"detail,omitempty"`
	Since    time.Time     `json:"since"`
	Until    *time.Time    `json:"until"`
	Next     *Slot         `json:"next"`
	NextFree *FreeSlot     `json:"next_free"`
	Override *Override     `json:"override"`          // active manual status, null in automatic mode
	Pomodoro *Pomodoro     `json:"pomodoro"`          // running pomodoro timer, null if none
	Message  string        `json:"message,omitempty"` // short text for the e-ink
	Updated  time.Time     `json:"updated"`
}

// SameAs reports whether two states differ only in the Updated timestamp.
func (s State) SameAs(o State) bool {
	return equalTimePtr(s.Until, o.Until) && equalSlot(s.Next, o.Next) && equalFree(s.NextFree, o.NextFree) &&
		s.Status == o.Status && s.Phase == o.Phase && s.Light == o.Light && s.Reason == o.Reason &&
		s.Detail == o.Detail && s.Since.Equal(o.Since) && s.Message == o.Message &&
		equalOverride(s.Override, o.Override) && equalPomodoro(s.Pomodoro, o.Pomodoro)
}

// Evaluate computes the state at now.
func Evaluate(now time.Time, in Inputs) State {
	loc := in.Location
	if loc == nil {
		loc = time.Local
	}
	now = now.In(loc)
	// Anchor the look-back window at a local midnight so that slots which
	// started before the window report a stable "since" instead of one
	// that moves with every evaluation.
	y, m, d := now.Add(-in.Rules.Lookback).Date()
	from := time.Date(y, m, d, 0, 0, 0, 0, loc)
	to := now.Add(in.Rules.Horizon)

	claims := Claims(now, from, to, in)
	raw := Resolve(claims, from, to)
	all := mergeByStatus(raw)
	planned := mergeByStatus(Resolve(onlyPlanned(claims), from, to))

	i := segmentAt(all, now)
	cur := all[i]
	st := State{
		Status:  cur.Status,
		Reason:  cur.Reason,
		Detail:  cur.Detail,
		Since:   cur.Start,
		Until:   endPtr(cur, to),
		Updated: now,
	}
	// The reason may change inside a merged slot (calendar → microphone);
	// report the reason that applies right now.
	if r := raw[segmentAt(raw, now)]; r.Status == cur.Status {
		st.Reason, st.Detail = r.Reason, r.Detail
	}

	if st.Until != nil && i+1 < len(all) {
		n := all[i+1]
		st.Next = &Slot{Status: n.Status, Reason: n.Reason, Since: n.Start, Until: endPtr(n, to)}
	}
	st.NextFree = nextFree(planned, st, now, to)
	if o := in.Override; o != nil && (o.Until.IsZero() || o.Until.After(now)) {
		st.Override = o
	}
	if p := in.Pomodoro; p != nil && (p.End().IsZero() || p.End().After(now)) {
		st.Pomodoro = p
	}
	st.Message = in.Message
	st.Phase = phase(st, now, in.Rules)
	st.Light = status.LightFor(st.Status, st.Phase)
	return st
}

// Timeline returns the planned slots (calendar, work hours, timers and
// overrides with a known end) in [from, to), ignoring live signals.
func Timeline(now, from, to time.Time, in Inputs) []Slot {
	if in.Location != nil {
		from, to = from.In(in.Location), to.In(in.Location)
	}
	segs := mergeByStatus(Resolve(onlyPlanned(Claims(now, from, to, in)), from, to))
	out := make([]Slot, 0, len(segs))
	for _, s := range segs {
		e := s.End
		out = append(out, Slot{Status: s.Status, Reason: s.Reason, Since: s.Start, Until: &e})
	}
	return out
}

// DayOff reports whether t falls on a day without working hours: a public
// holiday, an extra day off or an all-day out-of-office event (vacation).
func (in Inputs) DayOff(t time.Time) bool {
	if in.Holidays.IsHoliday(t) {
		return true
	}
	for _, e := range in.Events {
		if e.AllDay && e.Kind == calendar.KindOutOfOffice && !t.Before(e.Start) && t.Before(e.End) {
			return true
		}
	}
	return false
}

func phase(st State, now time.Time, r Rules) status.Phase {
	if st.Reason == ReasonCompanionLost {
		return status.PhaseFault
	}
	if st.Next != nil && st.Next.Status == status.DND {
		left := st.Next.Since.Sub(now)
		lastCall := r.LastCall.Or(StVOLastCall)
		switch {
		case st.Status == status.Free && left <= AmberTime:
			return status.PhaseAmber
		case st.Status == status.Free && left <= AmberTime+lastCall,
			st.Status == status.Working && left <= lastCall:
			return status.PhaseLastCall
		}
	}
	if st.Status == status.DND && st.Until != nil && st.Until.Sub(now) <= r.EndingSoon.Or(StVOEndingSoon) &&
		st.Next != nil && (st.Next.Status == status.Free || st.Next.Status == status.Working) {
		return status.PhaseEndingSoon
	}
	return status.PhaseNormal
}

func nextFree(planned []Segment, st State, now, horizon time.Time) *FreeSlot {
	if st.Status == status.Free {
		return &FreeSlot{Start: st.Since, End: st.Until}
	}
	// While a state of unknown length is active (a call without calendar
	// entry, a locked screen), nobody can say when the next free slot is.
	if st.Until == nil {
		return nil
	}
	ref := *st.Until
	for _, s := range planned {
		if s.Status != status.Free || !s.End.After(ref) {
			continue
		}
		start := s.Start
		if start.Before(ref) {
			start = ref
		}
		return &FreeSlot{Start: start, End: endPtr(s, horizon)}
	}
	return nil
}

func onlyPlanned(cs []Claim) []Claim {
	out := make([]Claim, 0, len(cs))
	for _, c := range cs {
		if c.planned {
			out = append(out, c)
		}
	}
	return out
}

func endPtr(s Segment, horizon time.Time) *time.Time {
	if !s.End.Before(horizon) {
		return nil
	}
	e := s.End
	return &e
}

func segmentAt(segs []Segment, t time.Time) int {
	i := sort.Search(len(segs), func(i int) bool { return segs[i].End.After(t) })
	if i == len(segs) {
		i = len(segs) - 1
	}
	return i
}

func equalTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func equalSlot(a, b *Slot) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Status == b.Status && a.Reason == b.Reason && a.Since.Equal(b.Since) && equalTimePtr(a.Until, b.Until)
}

func equalOverride(a, b *Override) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Status == b.Status && a.Start.Equal(b.Start) && a.Until.Equal(b.Until) &&
		a.UntilChange == b.UntilChange && a.Base == b.Base
}

func equalPomodoro(a, b *Pomodoro) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Start.Equal(b.Start) && a.Work == b.Work && a.Break == b.Break && a.Cycles == b.Cycles && a.WorkStatus == b.WorkStatus
}

func equalFree(a, b *FreeSlot) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Start.Equal(b.Start) && equalTimePtr(a.End, b.End)
}
