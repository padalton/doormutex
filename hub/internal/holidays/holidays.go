// Package holidays knows public holidays per country.
package holidays

import (
	"fmt"
	"strings"
	"time"
)

// Calendar answers whether a date is a day off.
type Calendar struct {
	country string
	extra   map[string]bool // YYYY-MM-DD
}

// New returns a holiday calendar for a country code ("AT", or "" for none)
// plus extra days off given as YYYY-MM-DD.
func New(country string, extra []string) (*Calendar, error) {
	c := &Calendar{country: strings.ToUpper(country), extra: map[string]bool{}}
	switch c.country {
	case "", "AT":
	default:
		return nil, fmt.Errorf("holidays: country %q not supported yet (supported: AT)", country)
	}
	for _, d := range extra {
		if _, err := time.Parse(time.DateOnly, d); err != nil {
			return nil, fmt.Errorf("holidays: extra day %q: %w", d, err)
		}
		c.extra[d] = true
	}
	return c, nil
}

// IsHoliday reports whether the calendar date of t (in t's location) is a day off.
func (c *Calendar) IsHoliday(t time.Time) bool {
	if c == nil {
		return false
	}
	if c.extra[t.Format(time.DateOnly)] {
		return true
	}
	if c.country == "AT" {
		_, ok := Austria(t.Year())[dateKey(t)]
		return ok
	}
	return false
}

// Name returns the holiday name for t, or "" if it is a regular day.
func (c *Calendar) Name(t time.Time) string {
	if c == nil {
		return ""
	}
	if c.country == "AT" {
		if n, ok := Austria(t.Year())[dateKey(t)]; ok {
			return n
		}
	}
	if c.extra[t.Format(time.DateOnly)] {
		return "day off"
	}
	return ""
}

type key struct {
	m time.Month
	d int
}

func dateKey(t time.Time) key { return key{t.Month(), t.Day()} }

// Austria returns the statutory public holidays in Austria for a year
// (Feiertagsruhegesetz). Good Friday is not a general holiday since 2019;
// 24 and 31 December are not statutory holidays.
func Austria(year int) map[key]string {
	e := Easter(year)
	rel := func(days int) key { return dateKey(e.AddDate(0, 0, days)) }
	return map[key]string{
		{time.January, 1}:   "Neujahr",
		{time.January, 6}:   "Heilige Drei Könige",
		rel(1):              "Ostermontag",
		{time.May, 1}:       "Staatsfeiertag",
		rel(39):             "Christi Himmelfahrt",
		rel(50):             "Pfingstmontag",
		rel(60):             "Fronleichnam",
		{time.August, 15}:   "Mariä Himmelfahrt",
		{time.October, 26}:  "Nationalfeiertag",
		{time.November, 1}:  "Allerheiligen",
		{time.December, 8}:  "Mariä Empfängnis",
		{time.December, 25}: "Christtag",
		{time.December, 26}: "Stefanitag",
	}
}

// Easter returns Easter Sunday (Gregorian calendar) using the
// anonymous Gregorian algorithm (Meeus/Jones/Butcher).
func Easter(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
