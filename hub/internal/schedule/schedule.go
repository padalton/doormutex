// Package schedule models the weekly working hours.
package schedule

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Range is a time-of-day interval, e.g. 08:00-12:00. End is exclusive.
type Range struct {
	Start, End time.Duration // offsets since local midnight
}

// ParseRange parses "HH:MM-HH:MM". "24:00" is allowed as an end.
func ParseRange(s string) (Range, error) {
	from, to, ok := strings.Cut(strings.TrimSpace(s), "-")
	if !ok {
		return Range{}, fmt.Errorf("schedule: %q: want HH:MM-HH:MM", s)
	}
	a, err := parseClock(from)
	if err != nil {
		return Range{}, fmt.Errorf("schedule: %q: %w", s, err)
	}
	b, err := parseClock(to)
	if err != nil {
		return Range{}, fmt.Errorf("schedule: %q: %w", s, err)
	}
	if b <= a {
		return Range{}, fmt.Errorf("schedule: %q: end must be after start", s)
	}
	return Range{a, b}, nil
}

func parseClock(s string) (time.Duration, error) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("bad time %q", s)
	}
	if h < 0 || m < 0 || m > 59 || h > 24 || (h == 24 && m != 0) {
		return 0, fmt.Errorf("bad time %q", s)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

// Week holds the working hours per weekday.
type Week map[time.Weekday][]Range

// ParseWeek parses a map like {"monday": ["08:00-12:00", "12:30-17:00"]}.
func ParseWeek(days map[string][]string) (Week, error) {
	names := map[string]time.Weekday{}
	for d := time.Sunday; d <= time.Saturday; d++ {
		names[strings.ToLower(d.String())] = d
	}
	w := Week{}
	for name, ranges := range days {
		d, ok := names[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("schedule: unknown weekday %q", name)
		}
		for _, r := range ranges {
			pr, err := ParseRange(r)
			if err != nil {
				return nil, err
			}
			w[d] = append(w[d], pr)
		}
		sort.Slice(w[d], func(i, j int) bool { return w[d][i].Start < w[d][j].Start })
		for i := 1; i < len(w[d]); i++ {
			if w[d][i].Start < w[d][i-1].End {
				return nil, fmt.Errorf("schedule: overlapping ranges on %s", name)
			}
		}
	}
	return w, nil
}

// Interval is an absolute time interval [Start, End).
type Interval struct {
	Start, End time.Time
}

// Intervals returns all working intervals overlapping [from, to) in loc,
// skipping days for which skip returns true (holidays). Intervals are
// clipped to [from, to).
func (w Week) Intervals(from, to time.Time, loc *time.Location, skip func(time.Time) bool) []Interval {
	var out []Interval
	day := time.Date(from.In(loc).Year(), from.In(loc).Month(), from.In(loc).Day(), 0, 0, 0, 0, loc)
	for ; day.Before(to); day = day.AddDate(0, 0, 1) {
		if skip != nil && skip(day.Add(12*time.Hour)) {
			continue
		}
		for _, r := range w[day.Weekday()] {
			// Build wall-clock times so DST days keep their local hours.
			s := atOffset(day, r.Start, loc)
			e := atOffset(day, r.End, loc)
			if !e.After(from) || !s.Before(to) {
				continue
			}
			if s.Before(from) {
				s = from
			}
			if e.After(to) {
				e = to
			}
			out = append(out, Interval{s, e})
		}
	}
	return out
}

func atOffset(day time.Time, off time.Duration, loc *time.Location) time.Time {
	h := int(off / time.Hour)
	m := int((off % time.Hour) / time.Minute)
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc)
}

// EndOfWorkday returns the end of the working interval containing t,
// or false if t is outside working hours.
func (w Week) EndOfWorkday(t time.Time, loc *time.Location, skip func(time.Time) bool) (time.Time, bool) {
	for _, iv := range w.Intervals(t.Add(-24*time.Hour), t.Add(24*time.Hour), loc, skip) {
		if !t.Before(iv.Start) && t.Before(iv.End) {
			return iv.End, true
		}
	}
	return time.Time{}, false
}
