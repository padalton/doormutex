// The light's own MQTT topics, telemetry document and Home Assistant
// discovery. The hub announces one HA device per profile; each light
// announces itself as a second device with battery, charging, signal and
// firmware, plus buttons to identify and restart it.
//
// Topics (base = the hub's mqtt.base_topic, id = "doormutex-<mac>"):
//
//   <base>/device/<id>/telemetry   retained JSON, see dm_telemetry_t
//   <base>/device/<id>/online      "online"/"offline", retained; "offline" is the Last Will
//   <base>/device/<id>/cmd         commands: identify | restart | ota <url>
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    char id[24];         // "doormutex-a1b2c3" (last three MAC bytes)
    const char *profile; // hub profile, e.g. "office"
    const char *base_topic;
    const char *discovery_prefix;
    const char *firmware; // version string
    const char *model;    // e.g. "ESP32-C6 traffic light"
} dm_device_t;

// dm_device_id formats "doormutex-xxxxxx" from a MAC address.
void dm_device_id(const uint8_t mac[6], char *out, size_t cap);

typedef struct {
    char telemetry[96], online[96], cmd[96];
    char state[96], hub_online[96], display[96];
} dm_topics_t;

// dm_topics fills the light's own topics and the hub topics it subscribes to.
void dm_topics(const dm_device_t *dev, dm_topics_t *t);

typedef struct {
    float battery_pct;   // < 0 = unknown (no fuel gauge)
    int battery_mv;      // <= 0 = unknown
    bool charging;
    bool usb;            // external power present
    int rssi;            // dBm, 0 = unknown
    const char *light;   // what the LEDs show
    const char *fault;   // failsafe reason, "none" if fine
    uint32_t uptime_s;
    const char *firmware;
} dm_telemetry_t;

// dm_telemetry_json writes the telemetry document; returns its length or
// 0 if it doesn't fit.
size_t dm_telemetry_json(const dm_telemetry_t *t, char *out, size_t cap);

// dm_discovery builds HA's device-based discovery message. Returns the
// payload length or 0 if out is too small.
size_t dm_discovery(const dm_device_t *dev, char *topic, size_t topic_cap, char *out, size_t cap);

#ifdef __cplusplus
}
#endif
