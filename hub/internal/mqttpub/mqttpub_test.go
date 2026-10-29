package mqttpub

import (
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/service"
	"github.com/padalton/doormutex/hub/internal/status"
)

// fakeCtl records the commands it receives.
type fakeCtl struct {
	mu       sync.Mutex
	override string // "auto" after ClearOverride
	message  *string
}

func (f *fakeCtl) SetOverride(r service.OverrideRequest) (engine.State, error) {
	if _, err := status.Parse(string(r.Status)); err != nil {
		return engine.State{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.override = string(r.Status) + "/" + r.Mode
	return engine.State{}, nil
}

func (f *fakeCtl) ClearOverride() engine.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.override = "auto"
	return engine.State{}
}

func (f *fakeCtl) SetMessage(text string, _ time.Duration) (engine.State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.message = &text
	return engine.State{}, nil
}

func (f *fakeCtl) get() (string, *string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.override, f.message
}

type msg struct{ topic, payload string }

func (m msg) Duplicate() bool   { return false }
func (m msg) Qos() byte         { return 1 }
func (m msg) Retained() bool    { return false }
func (m msg) Topic() string     { return m.topic }
func (m msg) MessageID() uint16 { return 0 }
func (m msg) Payload() []byte   { return []byte(m.payload) }
func (m msg) Ack()              {}

func TestDiscoveryPayload(t *testing.T) {
	topic, b := Discovery(Config{BaseTopic: "doormutex", DiscoveryPrefix: "homeassistant/", Version: "test"},
		Profile{ID: "teen-room", Name: "Teen room", Ctl: &fakeCtl{}})
	if topic != "homeassistant/device/doormutex_teen_room/config" {
		t.Fatalf("topic = %s", topic)
	}
	var p struct {
		Device       map[string]any            `json:"device"`
		Availability string                    `json:"availability_topic"`
		StateTopic   string                    `json:"state_topic"`
		Components   map[string]map[string]any `json:"components"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p.Availability != "doormutex/hub/online" || p.StateTopic != "doormutex/teen-room/state" || p.Device["name"] != "doormutex Teen room" {
		t.Errorf("topics/device: %+v", p)
	}
	want := map[string]string{
		"status":          "sensor.doormutex_teen_room_status",
		"status_changed":  "event.doormutex_teen_room_status_changed",
		"slot_start":      "sensor.doormutex_teen_room_slot_start",
		"slot_end":        "sensor.doormutex_teen_room_slot_end",
		"next_free_start": "sensor.doormutex_teen_room_next_free_start",
		"next_free_end":   "sensor.doormutex_teen_room_next_free_end",
		"override":        "select.doormutex_teen_room_override",
		"message":         "text.doormutex_teen_room_message",
	}
	if len(p.Components) != len(want) {
		t.Errorf("components: got %d, want %d", len(p.Components), len(want))
	}
	for key, entity := range want {
		c, ok := p.Components[key]
		if !ok {
			t.Errorf("missing component %s", key)
			continue
		}
		if c["default_entity_id"] != entity || c["unique_id"] == nil || c["platform"] == nil {
			t.Errorf("component %s: %v", key, c)
		}
	}
	if opts, _ := p.Components["status"]["options"].([]any); len(opts) != 4 {
		t.Errorf("status options: %v", opts)
	}
	if opts, _ := p.Components["override"]["options"].([]any); len(opts) != 5 || opts[0] != "auto" {
		t.Errorf("override options: %v", opts)
	}
	if p.Components["override"]["command_topic"] != "doormutex/teen-room/override/set" {
		t.Errorf("override command topic: %v", p.Components["override"]["command_topic"])
	}
}

func TestCommands(t *testing.T) {
	ctl := &fakeCtl{}
	c := &Client{cfg: Config{BaseTopic: "doormutex"}, log: slog.New(slog.DiscardHandler),
		profiles: map[string]*Publisher{"teen": {profile: Profile{ID: "teen", Ctl: ctl}}}}
	c.onCommand(nil, msg{"doormutex/teen/override/set", "dnd"})
	if o, _ := ctl.get(); o != "dnd/indefinite" {
		t.Errorf("override = %q", o)
	}
	c.onCommand(nil, msg{"doormutex/teen/override/set", "auto"})
	if o, _ := ctl.get(); o != "auto" {
		t.Errorf("override = %q", o)
	}
	c.onCommand(nil, msg{"doormutex/teen/override/set", "purple"}) // rejected, logged
	if o, _ := ctl.get(); o != "auto" {
		t.Errorf("invalid status applied: %q", o)
	}
	c.onCommand(nil, msg{"doormutex/teen/message/set", " Gaming until 21:00 "})
	if _, m := ctl.get(); m == nil || *m != "Gaming until 21:00" {
		t.Errorf("message = %v", m)
	}
	c.onCommand(nil, msg{"doormutex/office/override/set", "dnd"}) // unknown profile: ignored
}
