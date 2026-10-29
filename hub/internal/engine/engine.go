// Package engine decides the traffic light state. It is pure: given the
// inputs and a point in time it returns the state, so it can be tested
// without clocks, networks or databases.
//
// Every source contributes claims (status, priority, validity interval).
// For every moment the claim with the best (lowest) priority wins.
package engine

import (
	"sort"
	"time"

	"github.com/padalton/office-traffic-light/hub/internal/calendar"
	"github.com/padalton/office-traffic-light/hub/internal/holidays"
	"github.com/padalton/office-traffic-light/hub/internal/schedule"
	"github.com/padalton/office-traffic-light/hub/internal/status"
)

// Priority of a claim; lower wins. Mirrors the table in the README.
type Priority int

const (
	PrioManual    Priority = 1 // override, focus timer
	PrioMedia     Priority = 2 // microphone or camera in use
	PrioPresence  Priority = 3 // screen locked, companion silent
	PrioCalendar  Priority = 4
	PrioWorkHours Priority = 5
	PrioSignal    Priority = 6 // generic webhook signals
	PrioDefault   Priority = 7
)

// Reason codes reported in the state.
const (
	ReasonOverride         = "override"
	ReasonFocus            = "focus"
	ReasonFocusBreak       = "focus_break"
	ReasonMicrophone       = "microphone"
	ReasonCamera           = "camera"
	ReasonScreenLocked     = "screen_locked"
	ReasonCompanionOffline = "companion_offline"
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

// Override is a manual status. A zero Until means indefinite.
type Override struct {
	Status status.Status `json:"status"`
	Start  time.Time     `json:"start"`
	Until  time.Time     `json:"until,omitzero"`
}

// Focus is a pomodoro timer: Work, then Break, repeated Cycles times
// (0 = until stopped). WorkStatus is shown during work phases.
type Focus struct {
	Start      time.Time     `json:"start"`
	Work       time.Duration `json:"work"`
	Break      time.Duration `json:"break"`
	Cycles     int           `json:"cycles"`
	WorkStatus status.Status `json:"work_status"`
}

// End returns when the focus session ends (zero if unlimited).
func (f Focus) End() time.Time {
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

// Rules are the tunables of the decision logic.
type Rules struct {
	CalendarStatus   map[calendar.Kind]status.Status // missing kind → event ignored
	LastCall         time.Duration                   // green blinks this long before a meeting
	EndingSoon       time.Duration                   // red+yellow this long before a meeting ends
	MediaDebounce    time.Duration                   // mic/cam must be active this long
	MediaHold        time.Duration                   // keep red this long after mic/cam stop
	StaleAfter       time.Duration                   // companion silent this long → away
	RequireCompanion bool                            // no companion heartbeat → away
	Horizon          time.Duration                   // how far ahead to plan
	Lookback         time.Duration                   // how far back to find "since"
}

// DefaultRules returns the defaults documented in the README.
func DefaultRules() Rules {
	return Rules{
		CalendarStatus: map[calendar.Kind]status.Status{
			calendar.KindMeeting:     status.DND,
			calendar.KindFocus:       status.Working,
			calendar.KindOutOfOffice: status.Away,
			calendar.KindOther:       status.DND,
		},
		LastCall:         5 * time.Minute,
		EndingSoon:       5 * time.Minute,
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
	Focus     *Focus
	Companion Companion
	Events    []calendar.Event
	WorkWeek  schedule.Week
	Holidays  *holidays.Calendar
	Signals   []Signal
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
	Updated  time.Time     `json:"updated"`
}

// SameAs reports whether two states differ only in the Updated timestamp.
func (s State) SameAs(o State) bool {
	s.Updated, o.Updated = time.Time{}, time.Time{}
	return equalTimePtr(s.Until, o.Until) && equalSlot(s.Next, o.Next) && equalFree(s.NextFree, o.NextFree) &&
		s.Status == o.Status && s.Phase == o.Phase && s.Light == o.Light && s.Reason == o.Reason &&
		s.Detail == o.Detail && s.Since.Equal(o.Since)
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

func phase(st State, now time.Time, r Rules) status.Phase {
	switch {
	case st.Status == status.Free && st.Next != nil && st.Next.Status == status.DND &&
		st.Next.Since.Sub(now) <= r.LastCall:
		return status.PhaseLastCall
	case st.Status == status.DND && st.Until != nil && st.Until.Sub(now) <= r.EndingSoon &&
		st.Next != nil && (st.Next.Status == status.Free || st.Next.Status == status.Working):
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

func equalFree(a, b *FreeSlot) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Start.Equal(b.Start) && equalTimePtr(a.End, b.End)
}
