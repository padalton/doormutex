package engine

import (
	"sort"
	"time"

	"github.com/padalton/office-traffic-light/hub/internal/status"
)

// Claims collects the claims of all sources relevant for [from, to).
func Claims(now, from, to time.Time, in Inputs) []Claim {
	loc := in.Location
	if loc == nil {
		loc = time.Local
	}
	var cs []Claim

	// 1 – manual: override or focus timer.
	if o := in.Override; o != nil && (o.Until.IsZero() || o.Until.After(now)) {
		cs = append(cs, Claim{Status: o.Status, Priority: PrioManual, Reason: ReasonOverride,
			Start: o.Start, End: o.Until, planned: true})
	}
	if f := in.Focus; f != nil && f.Work > 0 {
		cs = append(cs, focusClaims(*f, from, to)...)
	}

	// 4 – calendar.
	for _, e := range in.Events {
		st, ok := in.Rules.CalendarStatus[e.Kind]
		if !ok || e.AllDay || e.Transparent || !e.End.After(e.Start) {
			continue
		}
		cs = append(cs, Claim{Status: st, Priority: PrioCalendar, Reason: ReasonCalendar,
			Detail: string(e.Kind), Start: e.Start, End: e.End, planned: true})
	}

	// 2 – microphone / camera (live).
	cs = append(cs, in.Companion.mediaClaims(now, in.Rules, calendarDNDEnd(cs, now))...)

	// 3 – presence (live).
	if c := in.Companion.presenceClaim(now, in.Rules); c != nil {
		cs = append(cs, *c)
	}

	// 5 – working hours.
	var skip func(time.Time) bool
	if in.Holidays != nil {
		skip = in.Holidays.IsHoliday
	}
	for _, iv := range in.WorkWeek.Intervals(from, to, loc, skip) {
		cs = append(cs, Claim{Status: status.Working, Priority: PrioWorkHours, Reason: ReasonWorkHours,
			Start: iv.Start, End: iv.End, planned: true})
	}

	// 6 – generic signals with TTL (live, but their end is known).
	for _, s := range in.Signals {
		if !s.Expires.After(now) {
			continue
		}
		cs = append(cs, Claim{Status: s.Status, Priority: PrioSignal, Reason: ReasonSignal,
			Detail: s.Name, Start: s.Start, End: s.Expires})
	}

	// 7 – default.
	cs = append(cs, Claim{Status: status.Free, Priority: PrioDefault, Reason: ReasonDefault,
		Start: from, End: to, planned: true})
	return cs
}

func focusClaims(f Focus, from, to time.Time) []Claim {
	var cs []Claim
	ws := f.WorkStatus
	if ws == "" {
		ws = status.Working
	}
	period := f.Work + f.Break
	for i, t := 0, f.Start; t.Before(to) && (f.Cycles <= 0 || i < f.Cycles); i, t = i+1, t.Add(period) {
		if !t.Add(period).After(from) {
			continue
		}
		cs = append(cs, Claim{Status: ws, Priority: PrioManual, Reason: ReasonFocus,
			Start: t, End: t.Add(f.Work), planned: true})
		if f.Break > 0 {
			cs = append(cs, Claim{Status: status.Free, Priority: PrioManual, Reason: ReasonFocusBreak,
				Start: t.Add(f.Work), End: t.Add(period), planned: true})
		}
		if period <= 0 {
			break
		}
	}
	return cs
}

// calendarDNDEnd returns the end of a calendar dnd claim covering now.
func calendarDNDEnd(cs []Claim, now time.Time) time.Time {
	var end time.Time
	for _, c := range cs {
		if c.Reason == ReasonCalendar && c.Status == status.DND &&
			!now.Before(c.Start) && now.Before(c.End) && c.End.After(end) {
			end = c.End
		}
	}
	return end
}

// Segment is a resolved interval [Start, End) with the winning claim.
type Segment struct {
	Start, End time.Time
	Status     status.Status
	Reason     string
	Detail     string
}

// Resolve splits [from, to) at every claim boundary and picks the winning
// claim per piece: lowest priority value, on a tie the most recent start.
func Resolve(cs []Claim, from, to time.Time) []Segment {
	points := []time.Time{from, to}
	for _, c := range cs {
		for _, t := range []time.Time{c.Start, c.End} {
			if !t.IsZero() && t.After(from) && t.Before(to) {
				points = append(points, t)
			}
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })

	var out []Segment
	for i := 0; i+1 < len(points); i++ {
		a, b := points[i], points[i+1]
		if !b.After(a) {
			continue
		}
		var win *Claim
		for j := range cs {
			c := &cs[j]
			if c.Start.After(a) || (!c.End.IsZero() && !c.End.After(a)) {
				continue
			}
			if win == nil || c.Priority < win.Priority || (c.Priority == win.Priority && c.Start.After(win.Start)) {
				win = c
			}
		}
		seg := Segment{Start: a, End: b, Status: status.Free, Reason: ReasonDefault}
		if win != nil {
			seg.Status, seg.Reason, seg.Detail = win.Status, win.Reason, win.Detail
		}
		if n := len(out); n > 0 && out[n-1].Status == seg.Status && out[n-1].Reason == seg.Reason &&
			out[n-1].Detail == seg.Detail {
			out[n-1].End = b
			continue
		}
		out = append(out, seg)
	}
	return out
}

// mergeByStatus joins neighbouring segments with the same status, keeping
// the reason of the first one.
func mergeByStatus(segs []Segment) []Segment {
	var out []Segment
	for _, s := range segs {
		if n := len(out); n > 0 && out[n-1].Status == s.Status {
			out[n-1].End = s.End
			continue
		}
		out = append(out, s)
	}
	return out
}
