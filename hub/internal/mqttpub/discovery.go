package mqttpub

import (
	"encoding/json"
	"strings"

	"github.com/padalton/doormutex/hub/internal/service"
	"github.com/padalton/doormutex/hub/internal/status"
)

const supportURL = "https://github.com/padalton/doormutex"

// EntityBase returns the prefix of a profile's entity IDs, e.g.
// "doormutex_office" → sensor.doormutex_office_status.
func EntityBase(profile string) string {
	return "doormutex_" + strings.ReplaceAll(profile, "-", "_")
}

// Discovery returns the topic and payload for Home Assistant's
// device-based MQTT discovery: one retained message per profile announcing
// its device with all entities. Battery and charging belong to the
// traffic light itself, which announces its own device.
func Discovery(cfg Config, p Profile) (string, []byte) {
	t := TopicsFor(cfg.BaseTopic, p.ID)
	base := EntityBase(p.ID)
	options := make([]string, 0, len(status.All))
	for _, s := range status.All {
		options = append(options, string(s))
	}
	entity := func(platform, key, name string, extra map[string]any) map[string]any {
		m := map[string]any{
			"platform":          platform,
			"unique_id":         base + "_" + key,
			"default_entity_id": platform + "." + base + "_" + key,
			"name":              name,
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	ts := func(key, name, tmpl string) map[string]any {
		return entity("sensor", key, name, map[string]any{"device_class": "timestamp", "value_template": tmpl})
	}
	components := map[string]any{
		"status": entity("sensor", "status", "Status", map[string]any{
			"icon":                  "mdi:traffic-light",
			"device_class":          "enum",
			"options":               options,
			"value_template":        "{{ value_json.status }}",
			"json_attributes_topic": t.State,
			"json_attributes_template": "{{ {'reason': value_json.reason, 'detail': value_json.detail, " +
				"'phase': value_json.phase, 'light': value_json.light, 'since': value_json.since, " +
				"'until': value_json.until, 'message': value_json.message} | tojson }}",
		}),
		"status_changed": entity("event", "status_changed", "Status changed", map[string]any{
			"state_topic": t.Event,
			"event_types": []string{"status_changed"},
		}),
		"slot_start": ts("slot_start", "Slot start", "{{ value_json.since }}"),
		// "None" makes HA show the sensor as unknown.
		"slot_end":        ts("slot_end", "Slot end", "{{ value_json.until or None }}"),
		"next_free_start": ts("next_free_start", "Next free start", "{{ value_json.next_free.start if value_json.next_free else None }}"),
		"next_free_end":   ts("next_free_end", "Next free end", "{{ value_json.next_free.end if value_json.next_free and value_json.next_free.end else None }}"),
	}
	if p.Ctl != nil {
		components["override"] = entity("select", "override", "Override", map[string]any{
			"icon":           "mdi:hand-back-right",
			"options":        append([]string{"auto"}, options...),
			"command_topic":  t.OverrideSet,
			"value_template": "{{ value_json.override.status if value_json.override else 'auto' }}",
		})
		components["message"] = entity("text", "message", "Message", map[string]any{
			"icon":           "mdi:message-text",
			"mode":           "text",
			"max":            service.MaxMessage,
			"command_topic":  t.MessageSet,
			"value_template": "{{ value_json.message | default('') }}",
		})
	}
	name := "doormutex"
	if p.Name != "" {
		name += " " + p.Name
	}
	payload := map[string]any{
		"device": map[string]any{
			"identifiers":  []string{base},
			"name":         name,
			"manufacturer": "doormutex",
			"model":        "Hub profile",
			"sw_version":   cfg.Version,
		},
		"origin": map[string]any{
			"name":        "doormutex hub",
			"sw_version":  cfg.Version,
			"support_url": supportURL,
		},
		"availability_topic": OnlineTopic(cfg.BaseTopic),
		"state_topic":        t.State,
		"qos":                qos,
		"components":         components,
	}
	b, _ := json.Marshal(payload)
	prefix := strings.TrimSuffix(cfg.DiscoveryPrefix, "/")
	return prefix + "/device/" + base + "/config", b
}
