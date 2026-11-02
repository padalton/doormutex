// The hub's state document (MQTT doormutex/<profile>/state, the same JSON
// as GET /api/v1/status) and the light's own failsafe.
//
// Principle: smart hub, dumb light. The hub decides status, phase and lamp
// pattern; the light only shows `light` – unless it can't trust the hub
// any more, then it blinks yellow by itself.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef enum {
    DM_STATUS_UNKNOWN = 0,
    DM_STATUS_FREE,    // green: come in
    DM_STATUS_WORKING, // yellow: only if it's important
    DM_STATUS_DND,     // red: do not disturb
    DM_STATUS_AWAY,    // off: not in the office
} dm_status_t;

typedef enum {
    DM_PHASE_UNKNOWN = 0,
    DM_PHASE_NORMAL,
    DM_PHASE_LAST_CALL,   // a meeting starts soon
    DM_PHASE_AMBER,       // yellow briefly before red
    DM_PHASE_ENDING_SOON, // red+yellow before the meeting ends
    DM_PHASE_FAULT,       // the companion went silent without signing off
} dm_phase_t;

// Lamp patterns – the same values as the hub's status.Light.
typedef enum {
    DM_LIGHT_OFF = 0,
    DM_LIGHT_RED,
    DM_LIGHT_YELLOW,
    DM_LIGHT_GREEN,
    DM_LIGHT_GREEN_BLINK,
    DM_LIGHT_RED_YELLOW,
    DM_LIGHT_RED_YELLOW_ALTERNATE,
    DM_LIGHT_YELLOW_BLINK, // fault – also the light's own failsafe
    DM_LIGHT_COUNT,
} dm_light_t;

// A message is at most 64 characters (hub: service.MaxMessage), i.e. up
// to 256 bytes of UTF-8.
#define DM_MESSAGE_MAX (64 * 4 + 1)

typedef struct {
    dm_status_t status;
    dm_phase_t phase;
    dm_light_t light;
    bool light_known; // false: missing or a value this firmware doesn't know
    char reason[32];
    char detail[32];
    int64_t since;           // Unix seconds, 0 = unknown
    int64_t until;           // 0 = until further notice
    int64_t next_free_start; // 0 = none
    int64_t next_free_end;   // 0 = open end or none
    int64_t updated;
    char message[DM_MESSAGE_MAX];
} dm_state_t;

// Parse errors.
#define DM_STATE_OK 0
#define DM_STATE_ERR_JSON -1   // not a JSON object
#define DM_STATE_ERR_FIELDS -2 // required fields (status, light) missing

// dm_state_parse fills out from the hub's JSON document. Unknown fields
// are ignored; an unknown light value parses fine but sets
// light_known = false (the failsafe then shows a fault).
int dm_state_parse(const char *json, size_t len, dm_state_t *out);

dm_light_t dm_light_parse(const char *s, bool *known);
const char *dm_light_name(dm_light_t light);
dm_status_t dm_status_parse(const char *s);
dm_phase_t dm_phase_parse(const char *s);

// ---- failsafe

typedef enum {
    DM_FAULT_NONE = 0,
    DM_FAULT_BOOTING,       // just started, no state yet: stay dark for the grace period
    DM_FAULT_NO_STATE,      // grace period over and still no state
    DM_FAULT_HUB_OFFLINE,   // the hub's Last Will: doormutex/hub/online = offline
    DM_FAULT_DISCONNECTED,  // Wi-Fi/MQTT down for longer than stale_after
    DM_FAULT_EXPIRED,       // `until` is long past and no new state came
    DM_FAULT_UNKNOWN_LIGHT, // the hub sent a pattern this firmware doesn't know
} dm_fault_t;

typedef struct {
    uint32_t boot_grace_ms;  // dark after boot until the first state (default 60 s)
    uint32_t stale_after_ms; // connection loss tolerated this long (default 120 s)
    uint32_t until_grace_s;  // `until` may be this far in the past (default 120 s)
} dm_failsafe_cfg_t;

#define DM_FAILSAFE_DEFAULTS {.boot_grace_ms = 60000, .stale_after_ms = 120000, .until_grace_s = 120}

// What the light knows about its link to the hub. Times in ms are from a
// monotonic clock (esp_timer); now_unix only counts if clock_valid (SNTP).
typedef struct {
    int64_t now_ms;
    int64_t boot_ms;
    bool have_state;
    bool hub_online_known;
    bool hub_online;
    bool connected;           // Wi-Fi and MQTT up
    int64_t disconnected_ms;  // when the connection was lost (valid if !connected)
    bool clock_valid;
    int64_t now_unix;
} dm_link_t;

dm_fault_t dm_failsafe_check(const dm_failsafe_cfg_t *cfg, const dm_link_t *link, const dm_state_t *state);

// dm_effective_light is what the LEDs show: the hub's pattern, dark while
// booting, yellow blinking on any fault.
dm_light_t dm_effective_light(dm_fault_t fault, const dm_state_t *state);

const char *dm_fault_name(dm_fault_t fault);

#ifdef __cplusplus
}
#endif
