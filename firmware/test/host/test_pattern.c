#include <stdint.h>

#include "check.h"
#include "dm_pattern.h"

void test_pattern(void)
{
    uint32_t next;
    // Steady patterns never change.
    struct {
        dm_light_t light;
        uint8_t mask;
    } steady[] = {
        {DM_LIGHT_OFF, 0},
        {DM_LIGHT_RED, DM_LAMP_RED},
        {DM_LIGHT_YELLOW, DM_LAMP_YELLOW},
        {DM_LIGHT_GREEN, DM_LAMP_GREEN},
        {DM_LIGHT_RED_YELLOW, DM_LAMP_RED | DM_LAMP_YELLOW},
    };
    for (size_t i = 0; i < sizeof(steady) / sizeof(steady[0]); i++) {
        const dm_pattern_t *p = dm_pattern_for(steady[i].light);
        CHECK_INT(dm_pattern_at(p, 0, &next), steady[i].mask);
        CHECK_INT(dm_pattern_at(p, 123456, &next), steady[i].mask);
        CHECK_INT(next, UINT32_MAX);
    }

    // Green blinks at 1 Hz and starts lit: four blinks in the StVO last call.
    const dm_pattern_t *gb = dm_pattern_for(DM_LIGHT_GREEN_BLINK);
    CHECK_INT(dm_pattern_at(gb, 0, &next), DM_LAMP_GREEN);
    CHECK_INT(next, 500);
    CHECK_INT(dm_pattern_at(gb, 499, &next), DM_LAMP_GREEN);
    CHECK_INT(next, 1);
    CHECK_INT(dm_pattern_at(gb, 500, &next), 0);
    CHECK_INT(next, 500);
    int blinks = 0;
    uint8_t prev = 0;
    for (uint32_t t = 0; t < DM_STVO_LAST_CALL_MS; t += 10) {
        uint8_t m = dm_pattern_at(gb, t, &next);
        blinks += (m && !prev);
        prev = m;
    }
    CHECK_INT(blinks, 4);

    // Fault: yellow blinks.
    const dm_pattern_t *yb = dm_pattern_for(DM_LIGHT_YELLOW_BLINK);
    CHECK_INT(dm_pattern_at(yb, 250, &next), DM_LAMP_YELLOW);
    CHECK_INT(dm_pattern_at(yb, 750, &next), 0);
    CHECK_INT(dm_pattern_at(yb, 1250, &next), DM_LAMP_YELLOW);

    // Red/yellow alternate: always exactly one lamp.
    const dm_pattern_t *ra = dm_pattern_for(DM_LIGHT_RED_YELLOW_ALTERNATE);
    CHECK_INT(dm_pattern_at(ra, 0, &next), DM_LAMP_RED);
    CHECK_INT(dm_pattern_at(ra, 600, &next), DM_LAMP_YELLOW);
    CHECK_INT(next, 400);
    CHECK_INT(dm_pattern_at(ra, 1000, &next), DM_LAMP_RED);

    // Out of range → the fault pattern.
    CHECK(dm_pattern_for(DM_LIGHT_COUNT) == yb);

    // Duty: quadratic brightness, compensation above the reference voltage.
    CHECK_INT(dm_led_duty(0.0f, 4200, 2000, 3000, 4095), 0);
    CHECK_INT(dm_led_duty(1.0f, 0, 2000, 3000, 4095), 4095);
    CHECK_INT(dm_led_duty(0.5f, 0, 2000, 3000, 4095), 1024);
    CHECK_INT(dm_led_duty(2.0f, 3000, 2000, 3000, 4095), 4095);
    // Red at a full cell: (3000-2000)/(4200-2000) = 0.4545…
    CHECK_INT(dm_led_duty(1.0f, 4200, 2000, 3000, 4095), 1861);
    // Below the reference nothing is boosted (can't go above full).
    CHECK_INT(dm_led_duty(1.0f, 2900, 2000, 3000, 4095), 4095);
    // A lit lamp never rounds to dark.
    CHECK_INT(dm_led_duty(0.001f, 4200, 2900, 3000, 4095), 1);
}
