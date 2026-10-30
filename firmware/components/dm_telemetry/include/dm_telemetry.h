// Battery and power: MAX17048 fuel gauge on I²C, the charger's status pins
// (BQ24074 CHG and PGOOD, both open-drain, active low).
#pragma once

#include <stdbool.h>

#include "esp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    int gpio_sda, gpio_scl; // I²C shared with the light sensor; -1 = no bus
    int gpio_chg;           // charger CHG, low while charging; -1 = not wired
    int gpio_pgood;         // charger PGOOD, low with USB power; -1 = not wired
} dm_power_cfg_t;

typedef struct {
    bool gauge;      // a fuel gauge answered
    float soc_pct;   // state of charge, valid if gauge
    int cell_mv;     // cell voltage, valid if gauge
    bool charging;
    bool usb;
} dm_power_t;

esp_err_t dm_power_init(const dm_power_cfg_t *cfg);
void dm_power_read(dm_power_t *out);

#ifdef __cplusplus
}
#endif
