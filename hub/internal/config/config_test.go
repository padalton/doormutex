package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/padalton/doormutex/hub/internal/calendar"
	"github.com/padalton/doormutex/hub/internal/status"
)

func TestExampleConfigLoads(t *testing.T) {
	t.Setenv("DOORMUTEX_MQTT_PASSWORD", "from-env")
	c, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	in, err := c.Inputs()
	if err != nil {
		t.Fatal(err)
	}
	if in.Location.String() != "Europe/Vienna" || len(in.WorkWeek[time.Friday]) != 1 || len(in.WorkWeek[time.Monday]) != 2 {
		t.Errorf("unexpected inputs: %v %v", in.Location, in.WorkWeek)
	}
	if !in.Holidays.IsHoliday(time.Date(2026, 12, 24, 12, 0, 0, 0, in.Location)) {
		t.Error("extra day off not applied")
	}
	if in.Rules.CalendarStatus[calendar.KindOther] != status.DND || in.Rules.MediaHold != 2*time.Minute {
		t.Errorf("rules: %+v", in.Rules)
	}
	if c.MQTT.Password != "from-env" || !c.DiscoveryEnabled() {
		t.Errorf("mqtt: %+v", c.MQTT)
	}
}

func TestInvalidConfig(t *testing.T) {
	for name, body := range map[string]string{
		"bad duration": "rules:\n  last_call: soon\n",
		"bad kind":     "rules:\n  calendar:\n    party: dnd\n",
		"bad status":   "rules:\n  calendar:\n    meeting: red\n",
		"bad hours":    "work_hours:\n  monday: [\"17:00-08:00\"]\n",
		"bad timezone": "timezone: Mars/Olympus\n",
	} {
		path := filepath.Join(t.TempDir(), "c.yaml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := Load(path)
		if err == nil {
			_, err = c.Inputs()
		}
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestIgnoreCalendarKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte("rules:\n  calendar:\n    other: ignore\n"), 0o600)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	in, err := c.Inputs()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := in.Rules.CalendarStatus[calendar.KindOther]; ok {
		t.Error("kind other should be ignored")
	}
}
