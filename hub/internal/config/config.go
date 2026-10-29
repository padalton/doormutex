// Package config loads the hub configuration from YAML.
package config

import (
	"fmt"
	"os"
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

// Config mirrors config.example.yaml.
type Config struct {
	Timezone string `yaml:"timezone"`
	Listen   string `yaml:"listen"`
	Database string `yaml:"database"`
	Holidays struct {
		Country string   `yaml:"country"`
		Extra   []string `yaml:"extra"`
	} `yaml:"holidays"`
	WorkHours map[string][]string `yaml:"work_hours"`
	Rules     struct {
		Calendar      map[string]string `yaml:"calendar"`
		LastCall      *Duration         `yaml:"last_call"`
		EndingSoon    *Duration         `yaml:"ending_soon"`
		MediaDebounce *Duration         `yaml:"media_debounce"`
		MediaHold     *Duration         `yaml:"media_hold"`
	} `yaml:"rules"`
	Presence struct {
		RequireCompanion *bool     `yaml:"require_companion"`
		StaleAfter       *Duration `yaml:"stale_after"`
	} `yaml:"presence"`
	Calendar struct {
		Provider     string           `yaml:"provider"` // "none" or "fake"
		PollInterval Duration         `yaml:"poll_interval"`
		FakeEvents   []calendar.Event `yaml:"fake_events"`
	} `yaml:"calendar"`
	MQTT struct {
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
	c.applyDefaults()
	c.MQTT.Password = os.Getenv("DOORMUTEX_MQTT_PASSWORD")
	if v := os.Getenv("DOORMUTEX_MQTT_URL"); v != "" {
		c.MQTT.URL = v
	}
	return c, nil
}

func (c *Config) applyDefaults() {
	def := func(s *string, v string) {
		if *s == "" {
			*s = v
		}
	}
	def(&c.Timezone, "Europe/Vienna")
	def(&c.Listen, ":8080")
	def(&c.Database, "/data/hub.db")
	def(&c.Holidays.Country, "AT")
	def(&c.Calendar.Provider, "none")
	def(&c.MQTT.ClientID, "doormutex-hub")
	def(&c.MQTT.BaseTopic, "doormutex")
	def(&c.MQTT.DiscoveryPrefix, "homeassistant")
	if c.Calendar.PollInterval == 0 {
		c.Calendar.PollInterval = Duration(2 * time.Minute)
	}
	if c.WorkHours == nil {
		mf := []string{"08:00-17:00"}
		c.WorkHours = map[string][]string{"monday": mf, "tuesday": mf, "wednesday": mf, "thursday": mf, "friday": mf}
	}
}

// DiscoveryEnabled reports whether Home Assistant discovery is on (default: yes).
func (c *Config) DiscoveryEnabled() bool { return c.MQTT.Discovery == nil || *c.MQTT.Discovery }

// Inputs builds the static part of the engine inputs.
func (c *Config) Inputs() (engine.Inputs, error) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return engine.Inputs{}, fmt.Errorf("timezone: %w", err)
	}
	week, err := schedule.ParseWeek(c.WorkHours)
	if err != nil {
		return engine.Inputs{}, err
	}
	hol, err := holidays.New(c.Holidays.Country, c.Holidays.Extra)
	if err != nil {
		return engine.Inputs{}, err
	}
	r := engine.DefaultRules()
	for kind, st := range c.Rules.Calendar {
		switch calendar.Kind(kind) {
		case calendar.KindMeeting, calendar.KindFocus, calendar.KindOutOfOffice, calendar.KindOther:
		default:
			return engine.Inputs{}, fmt.Errorf("rules.calendar: unknown event kind %q", kind)
		}
		if st == "ignore" {
			delete(r.CalendarStatus, calendar.Kind(kind))
			continue
		}
		s, err := status.Parse(st)
		if err != nil {
			return engine.Inputs{}, fmt.Errorf("rules.calendar.%s: %w", kind, err)
		}
		r.CalendarStatus[calendar.Kind(kind)] = s
	}
	set := func(dst *time.Duration, v *Duration) {
		if v != nil {
			*dst = time.Duration(*v)
		}
	}
	set(&r.LastCall, c.Rules.LastCall)
	set(&r.EndingSoon, c.Rules.EndingSoon)
	set(&r.MediaDebounce, c.Rules.MediaDebounce)
	set(&r.MediaHold, c.Rules.MediaHold)
	set(&r.StaleAfter, c.Presence.StaleAfter)
	if c.Presence.RequireCompanion != nil {
		r.RequireCompanion = *c.Presence.RequireCompanion
	}
	return engine.Inputs{Location: loc, Rules: r, WorkWeek: week, Holidays: hol}, nil
}
