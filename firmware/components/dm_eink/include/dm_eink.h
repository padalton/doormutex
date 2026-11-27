// The 1.54″ e-paper sign (200 × 200, SSD1681, SPI). The hub renders the
// picture; the light pushes the bits to the panel and draws its own battery
// into the footer (the hub's frames are per profile, see dm_battery.h).
//
// Buffer: 200 × 200 pixels, 1 bit each, row by row, the most significant
// bit is the leftmost pixel → 5000 bytes, 1 = white (the hub's eink.Pack).
//
// A full refresh per frame in a task of its own (about 2 s), the panel in
// deep sleep in between. TODO(firmware): partial refresh, if frames ever
// change often enough to make the full-refresh flash annoying.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

#define DM_EINK_WIDTH 200
#define DM_EINK_HEIGHT 200
#define DM_EINK_BYTES (DM_EINK_WIDTH * DM_EINK_HEIGHT / 8)

typedef struct {
    int gpio_mosi, gpio_sclk, gpio_cs, gpio_dc, gpio_rst, gpio_busy;
    // rotate_180: turn the picture, for a module mounted the other way.
    // With the init below, the picture is upright with the connector at the
    // top of the Waveshare module; connector at the bottom needs rotate_180.
    bool rotate_180;
} dm_eink_cfg_t;

esp_err_t dm_eink_init(const dm_eink_cfg_t *cfg);

// dm_eink_show takes a full frame from the hub. Returns
// ESP_ERR_INVALID_SIZE for anything but DM_EINK_BYTES; the same frame
// twice is skipped (no needless refresh).
esp_err_t dm_eink_show(const uint8_t *frame, size_t len);

// dm_eink_status describes the panel for the console: "not configured",
// "waiting for a frame", "ok (refresh 1389 ms)" or what went wrong.
const char *dm_eink_status(void);

// dm_eink_set_battery takes the fuel gauge's reading (percent < 0: no
// gauge, the corner stays empty). The sign is redrawn only in steps – see
// dm_battery_badge_update.
void dm_eink_set_battery(int percent, bool charging);

// dm_eink_battery is the percentage on the sign, or -1 if none is drawn.
int dm_eink_battery(void);

#ifdef __cplusplus
}
#endif
