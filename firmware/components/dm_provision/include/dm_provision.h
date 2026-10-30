// Setup over USB: settings in NVS, and one serial task that speaks both
// Improv (ESP Web Tools hands over Wi-Fi) and a small text console
// (MQTT broker, profile, brightness – see dm_config.h).
#pragma once

#include "dm_config.h"
#include "esp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

// NVS namespace "doormutex"; missing keys keep their defaults.
esp_err_t dm_settings_load(dm_config_t *c);
esp_err_t dm_settings_save(const dm_config_t *c);
esp_err_t dm_settings_erase(void);

typedef struct {
    const char *device_id; // "doormutex-a1b2c3", shown to ESP Web Tools
    const char *firmware;  // version
    void (*on_identify)(void);
} dm_serial_cfg_t;

// dm_serial_start installs the USB-Serial/JTAG driver (logs keep flowing
// through it) and starts the serial task. cfg is edited in place by the
// console and saved to NVS; MQTT changes apply after `restart`.
esp_err_t dm_serial_start(dm_config_t *cfg, const dm_serial_cfg_t *sc);

#ifdef __cplusplus
}
#endif
