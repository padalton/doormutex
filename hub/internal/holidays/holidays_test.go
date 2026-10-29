package holidays

import (
	"testing"
	"time"
)

func TestEaster(t *testing.T) {
	cases := map[int]string{
		2024: "2024-03-31",
		2025: "2025-04-20",
		2026: "2026-04-05",
		2027: "2027-03-28",
		2038: "2038-04-25",
	}
	for year, want := range cases {
		if got := Easter(year).Format(time.DateOnly); got != want {
			t.Errorf("Easter(%d) = %s, want %s", year, got, want)
		}
	}
}

func TestAustria(t *testing.T) {
	vienna, _ := time.LoadLocation("Europe/Vienna")
	cal, err := New("AT", []string{"2026-12-24"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		date string
		want bool
		name string
	}{
		{"2026-01-01", true, "Neujahr"},
		{"2026-01-06", true, "Heilige Drei Könige"},
		{"2026-04-03", false, ""}, // Good Friday is not a general holiday
		{"2026-04-06", true, "Ostermontag"},
		{"2026-05-01", true, "Staatsfeiertag"},
		{"2026-05-14", true, "Christi Himmelfahrt"},
		{"2026-05-25", true, "Pfingstmontag"},
		{"2026-06-04", true, "Fronleichnam"},
		{"2026-08-15", true, "Mariä Himmelfahrt"},
		{"2026-10-26", true, "Nationalfeiertag"},
		{"2026-10-27", false, ""},
		{"2026-11-01", true, "Allerheiligen"},
		{"2026-12-08", true, "Mariä Empfängnis"},
		{"2026-12-24", true, "day off"}, // extra day from config
		{"2026-12-25", true, "Christtag"},
		{"2026-12-26", true, "Stefanitag"},
		{"2026-12-31", false, ""},
	}
	for _, c := range cases {
		d, _ := time.ParseInLocation(time.DateOnly, c.date, vienna)
		d = d.Add(15 * time.Hour)
		if got := cal.IsHoliday(d); got != c.want {
			t.Errorf("IsHoliday(%s) = %v, want %v", c.date, got, c.want)
		}
		if got := cal.Name(d); got != c.name {
			t.Errorf("Name(%s) = %q, want %q", c.date, got, c.name)
		}
	}
}

func TestUnsupportedCountry(t *testing.T) {
	if _, err := New("XX", nil); err == nil {
		t.Fatal("expected error for unsupported country")
	}
	if _, err := New("AT", []string{"24.12.2026"}); err == nil {
		t.Fatal("expected error for malformed extra day")
	}
}
