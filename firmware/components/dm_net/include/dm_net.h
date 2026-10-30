// Wi-Fi station, MQTT and SNTP.
//
// Wi-Fi runs with modem power save (the radio sleeps between beacons, the
// CPU in automatic light sleep); MQTT keeps a permanent connection so a
// new state reaches the light in about a second.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "dm_config.h"
#include "dm_discovery.h"
#include "esp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    // A complete retained state document (chunked MQTT messages are
    // reassembled before the call).
    void (*on_state)(const char *json, size_t len);
    void (*on_hub_online)(bool online);
    // The e-ink bitmap from the hub, see dm_eink.h.
    void (*on_display)(const uint8_t *data, size_t len);
    void (*on_cmd)(const char *cmd, size_t len);
    // Wi-Fi and MQTT both up (true) or one of them lost (false).
    void (*on_link)(bool up);
} dm_net_callbacks_t;

// dm_net_init brings up netif, the event loop and the Wi-Fi driver (no
// connection yet).
esp_err_t dm_net_init(void);

// dm_net_has_credentials reports whether Wi-Fi credentials are stored.
bool dm_net_has_credentials(void);

// dm_net_try_wifi stores new credentials and connects; it blocks until an
// IP address arrives or timeout_ms passes (Improv provisioning).
esp_err_t dm_net_try_wifi(const char *ssid, const char *password, uint32_t timeout_ms);

// dm_net_start connects with the stored credentials and starts MQTT and
// SNTP. cfg and dev must stay valid.
esp_err_t dm_net_start(const dm_config_t *cfg, const dm_device_t *dev, const dm_net_callbacks_t *cb);

// dm_net_publish sends on a topic if MQTT is connected (QoS 1).
esp_err_t dm_net_publish(const char *topic, const char *payload, size_t len, bool retain);

bool dm_net_connected(void);
int dm_net_rssi(void); // dBm, 0 if unknown
bool dm_net_clock_valid(void);

// dm_net_scan lists visible networks for Improv (ssid, rssi, open?).
typedef struct {
    char ssid[33];
    int8_t rssi;
    bool secured;
} dm_net_ap_t;
int dm_net_scan(dm_net_ap_t *out, int max);

#ifdef __cplusplus
}
#endif
