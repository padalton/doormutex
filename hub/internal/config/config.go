// Package config loads the hub configuration from YAML.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/padalton/doormutex/hub/internal/calendar"
	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/holidays"
	"github.com/padalton/doormutex/hub/internal/schedule"
	"github.com/padalton/doormutex/hub/internal/status"
)

// Duration is a time.Duration written as "30s", "5m", "1h".
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*d = Duration(v)
	return nil
}

// Profile modes.
const (
	ModeOffice = "office" // desktop companion, calendar, working hours
	ModeRoom   = "room"   // controlled by phone, web UI or HA only
)

// Profile is one person and light.
type Profile struct {
	ID        string              `yaml:"id"`   // used in API, MQTT topics and HA entity IDs
	Name      string              `yaml:"name"` // shown in the web UI and HA
	Mode      string              `yaml:"mode"` // office (default) or room
	WorkHours map[string][]string `yaml:"work_hours"`
	Rules     struct {
		Calendar      map[string]string `yaml:"calendar"`
		LastCall      *engine.Lead      `yaml:"last_call"`   // "stvo" or a duration
		EndingSoon    *engine.Lead      `yaml:"ending_soon"` // "stvo" or a duration
		MediaDebounce *Duration         `yaml:"media_debounce"`
		MediaHold     *Duration         `yaml:"media_hold"`
		Default       string            `yaml:"default"` // status when nothing applies
	} `yaml:"rules"`
	Presence struct {
		RequireCompanion *bool     `yaml:"require_companion"`
		StaleAfter       *Duration `yaml:"stale_after"`
	} `yaml:"presence"`
	Calendar struct {
		Provider   string           `yaml:"provider"` // "none", "google" or "fake"
		Accounts   []GoogleAccount  `yaml:"accounts"` // provider google
		FakeEvents []calendar.Event `yaml:"fake_events"`
	} `yaml:"calendar"`
}

// GoogleAccount is one Google account of a profile, e.g. private or work.
type GoogleAccount struct {
	Name      string   `yaml:"name"`
	Calendars []string `yaml:"calendars"` // default: primary
	Mirror    string   `yaml:"mirror"`    // copies of a mirroring script: ignore (default) or use
}

// Google is the OAuth client shared by all profiles (a "desktop app"
// client from the Google Cloud console).
type Google struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"` // or DOORMUTEX_GOOGLE_CLIENT_SECRET
	MirrorKey    string `yaml:"mirror_key"`    // extendedProperties.private key of mirrored copies
}

// Config mirrors config.example.yaml. Without a profiles list, the
// top-level profile fields form a single profile "office".
type Config struct {
	Timezone string `yaml:"timezone"`
	Listen   string `yaml:"listen"`
	Database string `yaml:"database"`
	Holidays struct {
		Country string   `yaml:"country"`
		Extra   []string `yaml:"extra"`
	} `yaml:"holidays"`
	CalendarPoll Duration  `yaml:"calendar_poll"`
	Google       Google    `yaml:"google"`
	Single       Profile   `yaml:",inline"`
	Profiles     []Profile `yaml:"profiles"`
	MQTT         struct {
		URL             string `yaml:"url"`
		Username        string `yaml:"username"`
		Password        string `yaml:"-"` // only from DOORMUTEX_MQTT_PASSWORD
		ClientID        string `yaml:"client_id"`
		BaseTopic       string `yaml:"base_topic"`
		DiscoveryPrefix string `yaml:"discovery_prefix"`
		Discovery       *bool  `yaml:"discovery"`
	} `yaml:"mqtt"`
}

// Load reads a config file and applies defaults and environment overrides.
func Load(path string) (*Config, error) {
	c := &Config{}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(b, c); err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	}
	if err := c.applyDefaults(); err != nil {
		return nil, err
	}
	c.MQTT.Password = os.Getenv("DOORMUTEX_MQTT_PASSWORD")
	if v := os.Getenv("DOORMUTEX_GOOGLE_CLIENT_SECRET"); v != "" {
		c.Google.ClientSecret = v
	}
	if v := os.Getenv("DOORMUTEX_MQTT_URL"); v != "" {
		c.MQTT.URL = v
	}
	return c, nil
}

var profileID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func (c *Config) applyDefaults() error {
	def := func(s *string, v string) {
		if *s == "" {
			*s = v
		}
	}
	def(&c.Timezone, "Europe/Vienna")
	def(&c.Listen, ":8080")
	def(&c.Database, "/data/hub.db")
	def(&c.Holidays.Country, "AT")
	def(&c.MQTT.ClientID, "doormutex-hub")
	def(&c.MQTT.BaseTopic, "doormutex")
	def(&c.MQTT.DiscoveryPrefix, "homeassistant")
	if c.CalendarPoll == 0 {
		c.CalendarPoll = Duration(2 * time.Minute)
	}
	if len(c.Profiles) == 0 {
		p := c.Single
		def(&p.ID, "office")
		c.Profiles = []Profile{p}
	} else if c.Single.WorkHours != nil || c.Single.Calendar.Provider != "" || c.Single.Rules.Calendar != nil || c.Single.ID != "" {
		return errors.New("config: with a profiles list, put work_hours, rules, presence and calendar into the profiles")
	}
	seen := map[string]bool{}
	for i := range c.Profiles {
		p := &c.Profiles[i]
		if !profileID.MatchString(p.ID) {
			return fmt.Errorf("profiles[%d]: id %q: use lowercase letters, digits and '-' (max 32)", i, p.ID)
		}
		if seen[p.ID] {
			return fmt.Errorf("profiles: duplicate id %q", p.ID)
		}
		seen[p.ID] = true
		def(&p.Mode, ModeOffice)
		def(&p.Name, strings.ToUpper(p.ID[:1])+p.ID[1:])
		def(&p.Calendar.Provider, "none")
		if err := checkCalendar(p); err != nil {
			return err
		}
		switch p.Mode {
		case ModeOffice:
			if p.WorkHours == nil {
				mf := []string{"08:00-17:00"}
				p.WorkHours = map[string][]string{"monday": mf, "tuesday": mf, "wednesday": mf, "thursday": mf, "friday": mf}
			}
		case ModeRoom:
			if p.Presence.RequireCompanion == nil {
				no := false
				p.Presence.RequireCompanion = &no
			}
			def(&p.Rules.Default, string(status.Away))
		default:
			return fmt.Errorf("profile %s: mode %q: want office or room", p.ID, p.Mode)
		}
	}
	return nil
}

var accountName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func checkCalendar(p *Profile) error {
	switch p.Calendar.Provider {
	case "none", "fake":
		return nil
	case "google":
	default:
		return fmt.Errorf("profile %s: calendar.provider %q: want none, google or fake", p.ID, p.Calendar.Provider)
	}
	if len(p.Calendar.Accounts) == 0 {
		return fmt.Errorf("profile %s: calendar.accounts: add at least one Google account, e.g. - name: private", p.ID)
	}
	seen := map[string]bool{}
	for _, a := range p.Calendar.Accounts {
		if !accountName.MatchString(a.Name) || seen[a.Name] {
			return fmt.Errorf("profile %s: calendar account %q: unique lowercase name required", p.ID, a.Name)
		}
		seen[a.Name] = true
		if a.Mirror != "" && a.Mirror != "ignore" && a.Mirror != "use" {
			return fmt.Errorf("profile %s: calendar account %s: mirror must be ignore or use", p.ID, a.Name)
		}
	}
	return nil
}

// GoogleReady reports whether the OAuth client is configured.
func (c *Config) GoogleReady() error {
	if c.Google.ClientID == "" || c.Google.ClientSecret == "" {
		return errors.New("google.client_id and the client secret (google.client_secret or DOORMUTEX_GOOGLE_CLIENT_SECRET) are required – see docs/google-calendar.md")
	}
	return nil
}

// DiscoveryEnabled reports whether Home Assistant discovery is on (default: yes).
func (c *Config) DiscoveryEnabled() bool { return c.MQTT.Discovery == nil || *c.MQTT.Discovery }

// Inputs builds the static part of the engine inputs for a profile.
func (c *Config) Inputs(p Profile) (engine.Inputs, error) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return engine.Inputs{}, fmt.Errorf("timezone: %w", err)
	}
	week, err := schedule.ParseWeek(p.WorkHours)
	if err != nil {
		return engine.Inputs{}, fmt.Errorf("profile %s: %w", p.ID, err)
	}
	hol, err := holidays.New(c.Holidays.Country, c.Holidays.Extra)
	if err != nil {
		return engine.Inputs{}, err
	}
	r := engine.DefaultRules()
	for kind, st := range p.Rules.Calendar {
		if !slices.Contains(calendar.Kinds, calendar.Kind(kind)) {
			return engine.Inputs{}, fmt.Errorf("profile %s: rules.calendar: unknown event kind %q", p.ID, kind)
		}
		if st == "ignore" {
			delete(r.CalendarStatus, calendar.Kind(kind))
			continue
		}
		s, err := status.Parse(st)
		if err != nil {
			return engine.Inputs{}, fmt.Errorf("profile %s: rules.calendar.%s: %w", p.ID, kind, err)
		}
		r.CalendarStatus[calendar.Kind(kind)] = s
	}
	if p.Rules.LastCall != nil {
		r.LastCall = *p.Rules.LastCall
	}
	if p.Rules.EndingSoon != nil {
		r.EndingSoon = *p.Rules.EndingSoon
	}
	if p.Rules.Default != "" {
		if r.Default, err = status.Parse(p.Rules.Default); err != nil {
			return engine.Inputs{}, fmt.Errorf("profile %s: rules.default: %w", p.ID, err)
		}
	}
	set := func(dst *time.Duration, v *Duration) {
		if v != nil {
			*dst = time.Duration(*v)
		}
	}
	set(&r.MediaDebounce, p.Rules.MediaDebounce)
	set(&r.MediaHold, p.Rules.MediaHold)
	set(&r.StaleAfter, p.Presence.StaleAfter)
	if p.Presence.RequireCompanion != nil {
		r.RequireCompanion = *p.Presence.RequireCompanion
	}
	return engine.Inputs{Location: loc, Rules: r, WorkWeek: week, Holidays: hol}, nil
}

// Profile returns the profile with the given id.
func (c *Config) Profile(id string) (Profile, bool) {
	for _, p := range c.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}
