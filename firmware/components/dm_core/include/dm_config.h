// Device settings (stored in NVS) and the text console that sets them
// over USB: one command per line, e.g.
//
//   set mqtt_url mqtt://broker.local:1883
//   set profile office
//   show
//   restart
//
// Wi-Fi credentials are not part of it – they come via Improv (ESP Web
// Tools) and are kept by the Wi-Fi driver itself.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    char mqtt_url[128];        // mqtt://, mqtts://, ws:// or wss://
    char mqtt_user[64];
    char mqtt_pass[64];
    char profile[33];          // hub profile this light shows, e.g. "office"
    char base_topic[33];       // hub's mqtt.base_topic
    char discovery_prefix[33]; // Home Assistant discovery prefix
    char ntp[64];              // time server (for the `until` check)
    uint8_t brightness;        // 1…100 % until the light sensor takes over
} dm_config_t;

void dm_config_defaults(dm_config_t *c);

// dm_config_set changes one key. An empty value resets it to its default.
#define DM_CONFIG_OK 0
#define DM_CONFIG_UNKNOWN_KEY -1
#define DM_CONFIG_INVALID -2
int dm_config_set(dm_config_t *c, const char *key, const char *value);

// dm_config_ready reports whether everything needed to reach the hub is set.
bool dm_config_ready(const dm_config_t *c);

// Keys in a stable order (for NVS and `show`).
extern const char *const dm_config_keys[];
extern const size_t dm_config_key_count;

// dm_config_get formats one value; secrets come out as "***" if mask is set.
int dm_config_get(const dm_config_t *c, const char *key, bool mask, char *out, size_t cap);

typedef enum {
    DM_CMD_NONE = 0, // empty line
    DM_CMD_HELP,
    DM_CMD_SHOW,
    DM_CMD_SET,
    DM_CMD_RESTART,
    DM_CMD_FACTORY_RESET,
    DM_CMD_IDENTIFY,
    DM_CMD_UNKNOWN,
} dm_cmd_kind_t;

typedef struct {
    dm_cmd_kind_t kind;
    char key[24];
    char value[128];
} dm_cmd_t;

// dm_console_parse parses one line (trailing CR/LF allowed).
void dm_console_parse(const char *line, dm_cmd_t *cmd);

#ifdef __cplusplus
}
#endif
