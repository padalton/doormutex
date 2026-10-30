// Lamp patterns as step tables, and LED duty calculation.
//
// The hub schedules the phases (StVO timing or a lead time) and publishes
// a new `light` at each phase boundary; the light only has to render the
// pattern. The StVO constants below mirror hub/internal/engine for
// reference and documentation – the firmware doesn't schedule phases.
#pragma once

#include <stdint.h>

#include "dm_state.h"

#ifdef __cplusplus
extern "C" {
#endif

// StVO (Austrian road traffic regulations, § 38) – mirrored from the hub.
#define DM_STVO_LAST_CALL_MS 4000   // green blinks four times before yellow
#define DM_STVO_AMBER_MS 3000       // yellow before red
#define DM_STVO_ENDING_SOON_MS 2000 // red+yellow before green

// Blinking: 1 Hz, half on/half off – four blinks fill the StVO last call.
#define DM_BLINK_ON_MS 500
#define DM_BLINK_OFF_MS 500

// Lamp bits.
#define DM_LAMP_RED 0x1
#define DM_LAMP_YELLOW 0x2
#define DM_LAMP_GREEN 0x4

typedef struct {
    uint16_t ms;  // duration; 0 = forever (steady pattern)
    uint8_t mask; // DM_LAMP_* bits lit during this step
} dm_step_t;

typedef struct {
    const dm_step_t *steps;
    uint8_t count;
} dm_pattern_t;

// dm_pattern_for returns the step table of a lamp pattern. Every pattern
// starts with its "on" step, so a change becomes visible immediately.
const dm_pattern_t *dm_pattern_for(dm_light_t light);

// dm_pattern_at returns the lamps lit elapsed_ms after the pattern started
// and, in *next_ms, how long until the next change (UINT32_MAX if never).
uint8_t dm_pattern_at(const dm_pattern_t *p, uint32_t elapsed_ms, uint32_t *next_ms);

// dm_led_duty computes the PWM duty for one lamp.
//
//   brightness  0…1 from the light sensor / setting; perceived brightness
//               is roughly quadratic, so duty = brightness²
//   supply_mv   voltage feeding the LEDs (cell voltage); <= 0 = unknown,
//               no compensation
//   vf_mv       LED forward voltage (red ~2000, yellow ~2100, green ~2900)
//   ref_mv      supply voltage the series resistors are sized for – the
//               worst case near cut-off (3000 mV). Above it the current
//               rises roughly with (supply - vf), so the duty is scaled
//               down by (ref - vf) / (supply - vf) to keep the light even.
//   max_duty    full scale of the PWM timer
uint32_t dm_led_duty(float brightness, int supply_mv, int vf_mv, int ref_mv, uint32_t max_duty);

#ifdef __cplusplus
}
#endif
