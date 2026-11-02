#include "dm_pattern.h"

#define STEADY(m) {{.ms = 0, .mask = (m)}}
#define N(a) ((uint8_t)(sizeof(a) / sizeof((a)[0])))

static const dm_step_t off[] = STEADY(0);
static const dm_step_t red[] = STEADY(DM_LAMP_RED);
static const dm_step_t yellow[] = STEADY(DM_LAMP_YELLOW);
static const dm_step_t green[] = STEADY(DM_LAMP_GREEN);
static const dm_step_t red_yellow[] = STEADY(DM_LAMP_RED | DM_LAMP_YELLOW);
static const dm_step_t green_blink[] = {{DM_BLINK_ON_MS, DM_LAMP_GREEN}, {DM_BLINK_OFF_MS, 0}};
static const dm_step_t yellow_blink[] = {{DM_BLINK_ON_MS, DM_LAMP_YELLOW}, {DM_BLINK_OFF_MS, 0}};
// Last call from yellow: red and yellow take turns, never both and never dark.
static const dm_step_t red_yellow_alternate[] = {{DM_BLINK_ON_MS, DM_LAMP_RED}, {DM_BLINK_OFF_MS, DM_LAMP_YELLOW}};

static const dm_pattern_t patterns[DM_LIGHT_COUNT] = {
    [DM_LIGHT_OFF] = {off, N(off)},
    [DM_LIGHT_RED] = {red, N(red)},
    [DM_LIGHT_YELLOW] = {yellow, N(yellow)},
    [DM_LIGHT_GREEN] = {green, N(green)},
    [DM_LIGHT_GREEN_BLINK] = {green_blink, N(green_blink)},
    [DM_LIGHT_RED_YELLOW] = {red_yellow, N(red_yellow)},
    [DM_LIGHT_RED_YELLOW_ALTERNATE] = {red_yellow_alternate, N(red_yellow_alternate)},
    [DM_LIGHT_YELLOW_BLINK] = {yellow_blink, N(yellow_blink)},
};

const dm_pattern_t *dm_pattern_for(dm_light_t light)
{
    return light < DM_LIGHT_COUNT ? &patterns[light] : &patterns[DM_LIGHT_YELLOW_BLINK];
}

uint8_t dm_pattern_at(const dm_pattern_t *p, uint32_t elapsed_ms, uint32_t *next_ms)
{
    if (p->count == 1 || p->steps[0].ms == 0) {
        *next_ms = UINT32_MAX;
        return p->steps[0].mask;
    }
    uint32_t period = 0;
    for (uint8_t i = 0; i < p->count; i++) {
        period += p->steps[i].ms;
    }
    uint32_t t = elapsed_ms % period;
    for (uint8_t i = 0; i < p->count; i++) {
        if (t < p->steps[i].ms) {
            *next_ms = p->steps[i].ms - t;
            return p->steps[i].mask;
        }
        t -= p->steps[i].ms;
    }
    *next_ms = p->steps[0].ms; // not reached
    return p->steps[0].mask;
}

uint32_t dm_led_duty(float brightness, int supply_mv, int vf_mv, int ref_mv, uint32_t max_duty)
{
    if (brightness <= 0.0f) {
        return 0;
    }
    if (brightness > 1.0f) {
        brightness = 1.0f;
    }
    float duty = brightness * brightness;
    if (supply_mv > 0 && supply_mv > ref_mv && ref_mv > vf_mv) {
        duty *= (float)(ref_mv - vf_mv) / (float)(supply_mv - vf_mv);
    }
    uint32_t d = (uint32_t)(duty * (float)max_duty + 0.5f);
    // A lamp that should be on never rounds down to dark.
    return d == 0 ? 1 : d;
}
