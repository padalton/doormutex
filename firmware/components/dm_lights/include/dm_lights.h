// The three lamps: LEDC hardware PWM for dimming, a one-shot esp_timer
// that steps through the blink patterns of dm_pattern.h.
#pragma once

#include "dm_state.h"
#include "esp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    int gpio_red, gpio_yellow, gpio_green;
    int vf_mv_red, vf_mv_yellow, vf_mv_green; // forward voltages for the compensation
    int ref_mv;                               // supply the series resistors are sized for
} dm_lights_cfg_t;

esp_err_t dm_lights_init(const dm_lights_cfg_t *cfg);

// dm_lights_show switches to a pattern; the same pattern again keeps its
// rhythm (no visible restart on a repeated state).
void dm_lights_show(dm_light_t light);

// Brightness 0…1 (setting or light sensor), and the voltage feeding the
// LEDs for the compensation (<= 0: unknown).
void dm_lights_set_brightness(float brightness);
void dm_lights_set_supply_mv(int mv);

// dm_lights_identify cycles red → yellow → green for a few seconds, then
// returns to the current pattern ("which light is this?" from HA).
void dm_lights_identify(void);

dm_light_t dm_lights_current(void);

#ifdef __cplusplus
}
#endif
