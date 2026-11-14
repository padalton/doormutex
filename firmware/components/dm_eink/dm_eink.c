// SSD1681 driver for the Waveshare 1.54″ V2 e-paper module (200 × 200,
// black/white), written from the controller's datasheet: a full refresh
// per frame, the panel in deep sleep in between. The hub sends a frame only
// when the content changes, so a full refresh (about 2 s, one flash) fits.

#include "dm_eink.h"

#include <stdbool.h>
#include <stdio.h>
#include <string.h>

#include "driver/gpio.h"
#include "driver/spi_master.h"
#include "esp_check.h"
#include "esp_log.h"
#include "esp_rom_crc.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "freertos/task.h"

static const char *TAG = "eink";

#define SPI_HZ (4 * 1000 * 1000)
#define BUSY_TIMEOUT_MS 5000

// SSD1681 commands
#define CMD_DRIVER_OUTPUT 0x01
#define CMD_DEEP_SLEEP 0x10
#define CMD_DATA_ENTRY 0x11
#define CMD_SW_RESET 0x12
#define CMD_TEMP_SENSOR 0x18
#define CMD_ACTIVATE 0x20
#define CMD_UPDATE_CTRL2 0x22
#define CMD_WRITE_BW 0x24
#define CMD_BORDER 0x3C
#define CMD_RAM_X 0x44
#define CMD_RAM_Y 0x45
#define CMD_RAM_X_COUNTER 0x4E
#define CMD_RAM_Y_COUNTER 0x4F

static struct {
    dm_eink_cfg_t cfg;
    spi_device_handle_t spi;
    SemaphoreHandle_t mu;
    TaskHandle_t task;
    uint8_t frame[DM_EINK_BYTES]; // the next frame to draw, under mu
    bool pending;                 // under mu
    uint32_t shown_crc;           // the frame on the panel, under mu
    bool shown;                   // under mu
    bool ok;                      // the panel answered on the last refresh
    char status[48];
} e = {.status = "not configured"};

static void send(bool data, const uint8_t *buf, size_t len)
{
    if (len == 0) {
        return;
    }
    gpio_set_level(e.cfg.gpio_dc, data ? 1 : 0);
    spi_transaction_t t = {.length = len * 8, .tx_buffer = buf};
    ESP_ERROR_CHECK(spi_device_polling_transmit(e.spi, &t));
}

static void cmd(uint8_t c, const uint8_t *data, size_t len)
{
    send(false, &c, 1);
    send(true, data, len);
}

#define CMD(c, ...)                                                                                                    \
    do {                                                                                                               \
        const uint8_t d_[] = {__VA_ARGS__};                                                                            \
        cmd((c), d_, sizeof(d_));                                                                                      \
    } while (0)

// wait_busy waits until BUSY goes low (the controller is done) and returns
// how long it took in ms, or -1 on timeout.
static int wait_busy(void)
{
    int64_t start = esp_timer_get_time();
    while (gpio_get_level(e.cfg.gpio_busy) == 1) {
        if (esp_timer_get_time() - start > BUSY_TIMEOUT_MS * 1000LL) {
            return -1;
        }
        vTaskDelay(pdMS_TO_TICKS(10));
    }
    return (int)((esp_timer_get_time() - start) / 1000);
}

static bool panel_init(void)
{
    // Hardware reset wakes the panel from deep sleep, too.
    gpio_set_level(e.cfg.gpio_rst, 0);
    vTaskDelay(pdMS_TO_TICKS(10));
    gpio_set_level(e.cfg.gpio_rst, 1);
    vTaskDelay(pdMS_TO_TICKS(10));
    if (wait_busy() < 0) {
        return false;
    }
    cmd(CMD_SW_RESET, NULL, 0);
    if (wait_busy() < 0) {
        return false;
    }
    CMD(CMD_DRIVER_OUTPUT, 0xC7, 0x00, 0x01); // 200 gate lines, scan G199 → G0
    CMD(CMD_DATA_ENTRY, 0x01);                // X increment, Y decrement
    CMD(CMD_RAM_X, 0x00, 0x18);               // bytes 0..24 (200 px)
    CMD(CMD_RAM_Y, 0xC7, 0x00, 0x00, 0x00);   // lines 199..0
    CMD(CMD_BORDER, 0x01);                    // border follows white
    CMD(CMD_TEMP_SENSOR, 0x80);               // internal temperature sensor
    CMD(CMD_UPDATE_CTRL2, 0xB1);              // load temperature and waveform
    cmd(CMD_ACTIVATE, NULL, 0);
    if (wait_busy() < 0) {
        return false;
    }
    CMD(CMD_RAM_X_COUNTER, 0x00);
    CMD(CMD_RAM_Y_COUNTER, 0xC7, 0x00);
    return wait_busy() >= 0;
}

static void draw(const uint8_t *frame)
{
    int64_t start = esp_timer_get_time();
    if (!panel_init()) {
        ESP_LOGE(TAG, "panel not answering (BUSY stays high) – check wiring and power");
        snprintf(e.status, sizeof(e.status), "not answering (BUSY stays high)");
        e.ok = false;
        return;
    }
    // The hub packs 1 = white, as the black/white RAM expects.
    cmd(CMD_WRITE_BW, frame, DM_EINK_BYTES);
    CMD(CMD_UPDATE_CTRL2, 0xF7); // full update with the panel's own waveform
    cmd(CMD_ACTIVATE, NULL, 0);
    int refresh = wait_busy();
    CMD(CMD_DEEP_SLEEP, 0x01); // until the next reset; keeps the RAM
    if (refresh < 0) {
        ESP_LOGE(TAG, "refresh did not finish within %d ms", BUSY_TIMEOUT_MS);
        snprintf(e.status, sizeof(e.status), "refresh timed out");
        e.ok = false;
        return;
    }
    // A real refresh keeps BUSY high for a second or two; a few ms means
    // BUSY isn't connected or the controller ignored the command.
    if (refresh < 300) {
        ESP_LOGW(TAG, "refresh took only %d ms – is BUSY connected?", refresh);
    }
    e.ok = true;
    snprintf(e.status, sizeof(e.status), "ok (refresh %d ms)", refresh);
    ESP_LOGI(TAG, "panel refreshed (refresh %d ms, total %d ms)", refresh,
             (int)((esp_timer_get_time() - start) / 1000));
}

static uint8_t reverse_bits(uint8_t b)
{
    b = (uint8_t)((b & 0xF0) >> 4 | (b & 0x0F) << 4);
    b = (uint8_t)((b & 0xCC) >> 2 | (b & 0x33) << 2);
    return (uint8_t)((b & 0xAA) >> 1 | (b & 0x55) << 1);
}

static void eink_task(void *arg)
{
    (void)arg;
    static uint8_t buf[DM_EINK_BYTES]; // drawn outside the lock
    for (;;) {
        ulTaskNotifyTake(pdTRUE, portMAX_DELAY);
        xSemaphoreTake(e.mu, portMAX_DELAY);
        bool pending = e.pending;
        e.pending = false;
        if (pending && e.cfg.rotate_180) {
            // Last pixel first: the bytes backwards, the bits in each byte too.
            for (size_t i = 0; i < sizeof(buf); i++) {
                buf[i] = reverse_bits(e.frame[sizeof(buf) - 1 - i]);
            }
        } else if (pending) {
            memcpy(buf, e.frame, sizeof(buf));
        }
        xSemaphoreGive(e.mu);
        if (!pending) {
            continue;
        }
        draw(buf);
        if (!e.ok) {
            // Try the same frame again with the next one from the hub.
            xSemaphoreTake(e.mu, portMAX_DELAY);
            e.shown = false;
            xSemaphoreGive(e.mu);
        }
    }
}

esp_err_t dm_eink_init(const dm_eink_cfg_t *cfg)
{
    e.cfg = *cfg;
    if (cfg->gpio_mosi < 0 || cfg->gpio_sclk < 0 || cfg->gpio_cs < 0 || cfg->gpio_dc < 0 || cfg->gpio_rst < 0 ||
        cfg->gpio_busy < 0) {
        ESP_LOGI(TAG, "no e-ink panel configured");
        return ESP_OK;
    }
    gpio_config_t out = {.pin_bit_mask = (1ULL << cfg->gpio_dc) | (1ULL << cfg->gpio_rst), .mode = GPIO_MODE_OUTPUT};
    ESP_RETURN_ON_ERROR(gpio_config(&out), TAG, "dc/rst");
    gpio_set_level(cfg->gpio_rst, 1);
    gpio_config_t in = {.pin_bit_mask = 1ULL << cfg->gpio_busy, .mode = GPIO_MODE_INPUT};
    ESP_RETURN_ON_ERROR(gpio_config(&in), TAG, "busy");

    spi_bus_config_t bus = {
        .mosi_io_num = cfg->gpio_mosi,
        .miso_io_num = -1,
        .sclk_io_num = cfg->gpio_sclk,
        .quadwp_io_num = -1,
        .quadhd_io_num = -1,
        .max_transfer_sz = DM_EINK_BYTES,
    };
    ESP_RETURN_ON_ERROR(spi_bus_initialize(SPI2_HOST, &bus, SPI_DMA_CH_AUTO), TAG, "spi bus");
    spi_device_interface_config_t dev = {
        .clock_speed_hz = SPI_HZ,
        .mode = 0,
        .spics_io_num = cfg->gpio_cs,
        .queue_size = 1,
    };
    ESP_RETURN_ON_ERROR(spi_bus_add_device(SPI2_HOST, &dev, &e.spi), TAG, "spi device");
    e.mu = xSemaphoreCreateMutex();
    if (xTaskCreate(eink_task, "eink", 3072, NULL, 4, &e.task) != pdPASS) {
        return ESP_ERR_NO_MEM;
    }
    snprintf(e.status, sizeof(e.status), "waiting for a frame");
    ESP_LOGI(TAG, "SSD1681 on SPI (DIN %d, CLK %d, CS %d, DC %d, RST %d, BUSY %d)", cfg->gpio_mosi, cfg->gpio_sclk,
             cfg->gpio_cs, cfg->gpio_dc, cfg->gpio_rst, cfg->gpio_busy);
    return ESP_OK;
}

const char *dm_eink_status(void)
{
    return e.status;
}

esp_err_t dm_eink_show(const uint8_t *frame, size_t len)
{
    if (len != DM_EINK_BYTES) {
        ESP_LOGW(TAG, "frame of %u bytes ignored, want %d", (unsigned)len, DM_EINK_BYTES);
        return ESP_ERR_INVALID_SIZE;
    }
    if (e.task == NULL) {
        return ESP_ERR_INVALID_STATE;
    }
    uint32_t crc = esp_rom_crc32_le(0, frame, len);
    xSemaphoreTake(e.mu, portMAX_DELAY);
    if (e.shown && crc == e.shown_crc) {
        xSemaphoreGive(e.mu);
        return ESP_OK; // the same frame again, e.g. after a reconnect
    }
    memcpy(e.frame, frame, len);
    e.pending = true;
    e.shown = true;
    e.shown_crc = crc;
    xSemaphoreGive(e.mu);
    ESP_LOGI(TAG, "new frame (crc %08lx)", (unsigned long)crc);
    xTaskNotifyGive(e.task);
    return ESP_OK;
}
