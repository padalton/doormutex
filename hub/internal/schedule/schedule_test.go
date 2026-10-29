package schedule

import (
	"testing"
	"time"
)

func TestParseRange(t *testing.T) {
	good := []string{"08:00-12:00", " 9:30 - 17:15 ", "00:00-24:00"}
	for _, s := range good {
		if _, err := ParseRange(s); err != nil {
			t.Errorf("ParseRange(%q): %v", s, err)
		}
	}
	bad := []string{"8-12", "12:00-08:00", "08:00-08:00", "25:00-26:00", "08:60-09:00", "08:00"}
	for _, s := range bad {
		if _, err := ParseRange(s); err == nil {
			t.Errorf("ParseRange(%q): expected error", s)
		}
	}
}

func TestParseWeekRejectsOverlapAndUnknownDay(t *testing.T) {
	if _, err := ParseWeek(map[string][]string{"monday": {"08:00-12:00", "11:00-13:00"}}); err == nil {
		t.Error("expected overlap error")
	}
	if _, err := ParseWeek(map[string][]string{"montag": {"08:00-12:00"}}); err == nil {
		t.Error("expected unknown weekday error")
	}
}

func TestIntervalsAcrossDSTAndHolidays(t *testing.T) {
	vienna, _ := time.LoadLocation("Europe/Vienna")
	w, err := ParseWeek(map[string][]string{
		"monday": {"08:00-12:00", "12:30-17:00"}, "tuesday": {"08:00-17:00"},
		"wednesday": {"08:00-17:00"}, "thursday": {"08:00-17:00"}, "friday": {"08:00-13:00"},
		"sunday": {"10:00-11:00"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Week of the switch from CEST to CET (Sunday 2026-10-25), Monday 2026-10-26 is a holiday.
	from := time.Date(2026, 10, 25, 0, 0, 0, 0, vienna)
	to := from.AddDate(0, 0, 3)
	skip := func(d time.Time) bool { return d.Month() == time.October && d.Day() == 26 }
	got := w.Intervals(from, to, vienna, skip)
	want := []Interval{
		{time.Date(2026, 10, 25, 10, 0, 0, 0, vienna), time.Date(2026, 10, 25, 11, 0, 0, 0, vienna)},
		{time.Date(2026, 10, 27, 8, 0, 0, 0, vienna), time.Date(2026, 10, 27, 17, 0, 0, 0, vienna)},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Start.Equal(want[i].Start) || !got[i].End.Equal(want[i].End) {
			t.Errorf("interval %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestEndOfWorkday(t *testing.T) {
	vienna, _ := time.LoadLocation("Europe/Vienna")
	w, _ := ParseWeek(map[string][]string{"wednesday": {"08:00-12:00", "12:30-17:00"}})
	at := time.Date(2026, 10, 28, 14, 0, 0, 0, vienna)
	end, ok := w.EndOfWorkday(at, vienna, nil)
	if !ok || !end.Equal(time.Date(2026, 10, 28, 17, 0, 0, 0, vienna)) {
		t.Errorf("EndOfWorkday = %v %v", end, ok)
	}
	if _, ok := w.EndOfWorkday(at.Add(5*time.Hour), vienna, nil); ok {
		t.Error("expected no workday at 19:00")
	}
}
