// Battery, power and ambient light: MAX17048 fuel gauge and VEML7700 light
// sensor on I²C, the charger's status pins (BQ24074 CHG and PGOOD, both
// open-drain, active low).
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

// VEML7700 light sensor on the same I²C bus; call after dm_power_init.
// ESP_ERR_NOT_FOUND if none answers.
esp_err_t dm_ambient_init(void);
// dm_ambient_lux reads the ambient light; false while there is no sensor
// or no valid reading (e.g. right after a range switch).
bool dm_ambient_lux(float *lux);
// dm_ambient_status: "VEML7700", or what was found on the bus instead.
const char *dm_ambient_status(void);

#ifdef __cplusplus
}
#endif
