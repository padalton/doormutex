package mqttpub

import (
	"encoding/json"
	"testing"
)

func TestDiscoveryPayload(t *testing.T) {
	topic, b := Discovery(Config{BaseTopic: "doormutex", DiscoveryPrefix: "homeassistant/", Version: "test"})
	if topic != "homeassistant/device/doormutex_hub/config" {
		t.Fatalf("topic = %s", topic)
	}
	var p struct {
		Availability string                    `json:"availability_topic"`
		StateTopic   string                    `json:"state_topic"`
		Components   map[string]map[string]any `json:"components"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p.Availability != "doormutex/hub/online" || p.StateTopic != "doormutex/state" {
		t.Errorf("topics: %+v", p)
	}
	want := map[string]string{
		"status":          "sensor.doormutex_status",
		"status_changed":  "event.doormutex_status_changed",
		"slot_start":      "sensor.doormutex_slot_start",
		"slot_end":        "sensor.doormutex_slot_end",
		"next_free_start": "sensor.doormutex_next_free_start",
		"next_free_end":   "sensor.doormutex_next_free_end",
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
	opts, _ := p.Components["status"]["options"].([]any)
	if len(opts) != 4 {
		t.Errorf("status options: %v", opts)
	}
}
