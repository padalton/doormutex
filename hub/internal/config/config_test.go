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
	in, err := c.Inputs(c.Profiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if in.Location.String() != "Europe/Vienna" || len(in.WorkWeek[time.Friday]) != 1 || len(in.WorkWeek[time.Monday]) != 2 {
		t.Errorf("unexpected inputs: %v %v", in.Location, in.WorkWeek)
	}
	if !in.Holidays.IsHoliday(time.Date(2026, 10, 26, 12, 0, 0, 0, in.Location)) {
		t.Error("Nationalfeiertag missing")
	}
	if in.Rules.CalendarStatus[calendar.KindOther] != status.Working || in.Rules.CalendarStatus[calendar.KindOutOfOffice] != status.DND ||
		in.Rules.MediaHold != 2*time.Minute || in.Rules.LastCall != 0 || in.Rules.EndingSoon != 0 {
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
		"bad status":   "rules:\n  calendar:\n    video_call: red\n",
		"bad hours":    "work_hours:\n  monday: [\"17:00-08:00\"]\n",
		"bad timezone": "timezone: Mars/Olympus\n",
		"bad mode":     "profiles:\n  - id: x\n    mode: garage\n",
		"bad id":       "profiles:\n  - id: Office\n",
		"duplicate id": "profiles:\n  - id: a\n  - id: a\n",
		"mixed":        "work_hours: {}\nprofiles:\n  - id: a\n",
		"bad default":  "rules:\n  default: purple\n",
	} {
		path := filepath.Join(t.TempDir(), "c.yaml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := Load(path)
		if err == nil {
			_, err = c.Inputs(c.Profiles[0])
		}
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLeadTimes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte("rules:\n  last_call: 3m\n  ending_soon: stvo\n"), 0o600)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	in, err := c.Inputs(c.Profiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if time.Duration(in.Rules.LastCall) != 3*time.Minute || in.Rules.EndingSoon != 0 {
		t.Errorf("lead times: %v %v", in.Rules.LastCall, in.Rules.EndingSoon)
	}
}

func TestIgnoreCalendarKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte("rules:\n  calendar:\n    other: ignore\n"), 0o600)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	in, err := c.Inputs(c.Profiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := in.Rules.CalendarStatus[calendar.KindOther]; ok {
		t.Error("kind other should be ignored")
	}
}

func TestProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte(`
profiles:
  - id: office
    rules:
      last_call: 5m
  - id: teen
    name: Teen room
    mode: room
`), 0o600)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Profiles) != 2 {
		t.Fatalf("profiles: %+v", c.Profiles)
	}
	office, _ := c.Profile("office")
	in, err := c.Inputs(office)
	if err != nil {
		t.Fatal(err)
	}
	if !in.Rules.RequireCompanion || len(in.WorkWeek[time.Monday]) != 1 || time.Duration(in.Rules.LastCall) != 5*time.Minute || office.Name != "Office" {
		t.Errorf("office: %+v %+v", office, in.Rules)
	}
	teen, _ := c.Profile("teen")
	in, err = c.Inputs(teen)
	if err != nil {
		t.Fatal(err)
	}
	if in.Rules.RequireCompanion || len(in.WorkWeek[time.Monday]) != 0 || in.Rules.Default != status.Away || teen.Name != "Teen room" {
		t.Errorf("room: %+v %+v", teen, in.Rules)
	}
}

func TestSingleProfileFallback(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Profiles) != 1 || c.Profiles[0].ID != "office" || c.Profiles[0].Mode != ModeOffice {
		t.Fatalf("profiles: %+v", c.Profiles)
	}
}

func TestGoogleCalendarConfig(t *testing.T) {
	t.Setenv("DOORMUTEX_GOOGLE_CLIENT_SECRET", "from-env")
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte(`
google:
  client_id: abc.apps.googleusercontent.com
profiles:
  - id: office
    calendar:
      provider: google
      accounts:
        - name: private
        - name: work
          calendars: [primary, team@example.com]
          mirror: use
`), 0o600)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.GoogleReady(); err != nil || c.Google.ClientSecret != "from-env" {
		t.Fatalf("google: %+v %v", c.Google, err)
	}
	if a := c.Profiles[0].Calendar.Accounts; len(a) != 2 || a[1].Mirror != "use" || len(a[1].Calendars) != 2 {
		t.Fatalf("accounts: %+v", a)
	}
	for name, body := range map[string]string{
		"no accounts":  "profiles:\n  - id: a\n    calendar:\n      provider: google\n",
		"dup account":  "profiles:\n  - id: a\n    calendar:\n      provider: google\n      accounts: [{name: x}, {name: x}]\n",
		"bad mirror":   "profiles:\n  - id: a\n    calendar:\n      provider: google\n      accounts: [{name: x, mirror: maybe}]\n",
		"bad provider": "profiles:\n  - id: a\n    calendar:\n      provider: outlook\n",
	} {
		_ = os.WriteFile(path, []byte(body), 0o600)
		if _, err := Load(path); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
