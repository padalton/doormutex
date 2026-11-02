#include "dm_eink.h"

#include <stdbool.h>
#include <string.h>

#include "esp_log.h"
#include "esp_rom_crc.h"

static const char *TAG = "eink";

static struct {
    dm_eink_cfg_t cfg;
    uint32_t last_crc;
    bool have_frame;
} e;

esp_err_t dm_eink_init(const dm_eink_cfg_t *cfg)
{
    e.cfg = *cfg;
    ESP_LOGW(TAG, "panel driver not implemented yet – frames are only checked and logged");
    return ESP_OK;
}

esp_err_t dm_eink_show(const uint8_t *frame, size_t len)
{
    if (len != DM_EINK_BYTES) {
        ESP_LOGW(TAG, "frame of %u bytes ignored, want %d", (unsigned)len, DM_EINK_BYTES);
        return ESP_ERR_INVALID_SIZE;
    }
    uint32_t crc = esp_rom_crc32_le(0, frame, len);
    if (e.have_frame && crc == e.last_crc) {
        return ESP_OK;
    }
    e.last_crc = crc;
    e.have_frame = true;
    ESP_LOGI(TAG, "new frame (crc %08lx) – would refresh the panel now", (unsigned long)crc);
    return ESP_OK;
}
