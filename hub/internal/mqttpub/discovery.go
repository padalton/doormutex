package mqttpub

import (
	"encoding/json"
	"strings"

	"github.com/padalton/office-traffic-light/hub/internal/status"
)

const (
	deviceID   = "office_traffic_light_hub"
	entityBase = "office_traffic_light"
	supportURL = "https://github.com/padalton/office-traffic-light"
)

// Discovery returns the topic and payload for Home Assistant's
// device-based MQTT discovery: one retained message announcing the hub
// device with all its entities. Battery and charging belong to the
// traffic light itself, which announces its own device.
func Discovery(cfg Config) (string, []byte) {
	t := TopicsFor(cfg.BaseTopic)
	options := make([]string, 0, len(status.All))
	for _, s := range status.All {
		options = append(options, string(s))
	}
	ts := func(key, name, tmpl string) map[string]any {
		return map[string]any{
			"platform":          "sensor",
			"unique_id":         entityBase + "_" + key,
			"default_entity_id": "sensor." + entityBase + "_" + key,
			"name":              name,
			"device_class":      "timestamp",
			"value_template":    tmpl,
		}
	}
	payload := map[string]any{
		"device": map[string]any{
			"identifiers":  []string{deviceID},
			"name":         "Office Traffic Light",
			"manufacturer": "office-traffic-light",
			"model":        "Hub",
			"sw_version":   cfg.Version,
		},
		"origin": map[string]any{
			"name":        "office-traffic-light hub",
			"sw_version":  cfg.Version,
			"support_url": supportURL,
		},
		"availability_topic": t.Online,
		"state_topic":        t.State,
		"qos":                qos,
		"components": map[string]any{
			"status": map[string]any{
				"platform":              "sensor",
				"unique_id":             entityBase + "_status",
				"default_entity_id":     "sensor." + entityBase + "_status",
				"name":                  "Status",
				"icon":                  "mdi:traffic-light",
				"device_class":          "enum",
				"options":               options,
				"value_template":        "{{ value_json.status }}",
				"json_attributes_topic": t.State,
				"json_attributes_template": "{{ {'reason': value_json.reason, 'detail': value_json.detail, " +
					"'phase': value_json.phase, 'light': value_json.light, 'since': value_json.since, " +
					"'until': value_json.until} | tojson }}",
			},
			"status_changed": map[string]any{
				"platform":          "event",
				"unique_id":         entityBase + "_status_changed",
				"default_entity_id": "event." + entityBase + "_status_changed",
				"name":              "Status changed",
				"state_topic":       t.Event,
				"event_types":       []string{"status_changed"},
			},
			"slot_start": ts("slot_start", "Slot start", "{{ value_json.since }}"),
			// "None" makes HA show the sensor as unknown.
			"slot_end":        ts("slot_end", "Slot end", "{{ value_json.until or None }}"),
			"next_free_start": ts("next_free_start", "Next free start", "{{ value_json.next_free.start if value_json.next_free else None }}"),
			"next_free_end":   ts("next_free_end", "Next free end", "{{ value_json.next_free.end if value_json.next_free and value_json.next_free.end else None }}"),
		},
	}
	b, _ := json.Marshal(payload)
	prefix := strings.TrimSuffix(cfg.DiscoveryPrefix, "/")
	return prefix + "/device/" + deviceID + "/config", b
}
