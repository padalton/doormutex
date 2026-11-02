// The 1.54″ e-paper sign (200 × 200, SSD1681, SPI) – interface only for
// now. The hub renders the whole picture; the light just pushes the bits
// to the panel.
//
// Buffer: 200 × 200 pixels, 1 bit each, row by row, the most significant
// bit is the leftmost pixel → 5000 bytes. The polarity (1 = white or
// black) follows the hub's renderer; the driver inverts if needed.
//
// TODO(firmware): SPI driver for the SSD1681 (Waveshare 1.54″ V2): init
// sequence, full refresh, partial refresh for the "updated" time, a full
// refresh every N partials against ghosting, deep sleep of the panel
// between updates. Busy pin instead of fixed delays.
#pragma once

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
} dm_eink_cfg_t;

esp_err_t dm_eink_init(const dm_eink_cfg_t *cfg);

// dm_eink_show takes a full frame from the hub. Returns
// ESP_ERR_INVALID_SIZE for anything but DM_EINK_BYTES; the same frame
// twice is skipped (no needless refresh).
esp_err_t dm_eink_show(const uint8_t *frame, size_t len);

#ifdef __cplusplus
}
#endif
