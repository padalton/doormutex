#include "dm_telemetry.h"

#include "driver/gpio.h"
#include "driver/i2c_master.h"
#include "esp_check.h"
#include "esp_log.h"

static const char *TAG = "power";

#define MAX17048_ADDR 0x36
#define REG_VCELL 0x02 // 78.125 µV per bit
#define REG_SOC 0x04   // 1/256 % per bit
#define I2C_TIMEOUT_MS 50

static struct {
    dm_power_cfg_t cfg;
    i2c_master_bus_handle_t bus;
    i2c_master_dev_handle_t gauge;
} p;

esp_err_t dm_power_init(const dm_power_cfg_t *cfg)
{
    p.cfg = *cfg;
    uint64_t pins = 0;
    if (cfg->gpio_chg >= 0) {
        pins |= 1ULL << cfg->gpio_chg;
    }
    if (cfg->gpio_pgood >= 0) {
        pins |= 1ULL << cfg->gpio_pgood;
    }
    if (pins) {
        gpio_config_t io = {.pin_bit_mask = pins, .mode = GPIO_MODE_INPUT, .pull_up_en = GPIO_PULLUP_ENABLE};
        ESP_RETURN_ON_ERROR(gpio_config(&io), TAG, "charger pins");
    }
    if (cfg->gpio_sda < 0 || cfg->gpio_scl < 0) {
        return ESP_OK;
    }
    i2c_master_bus_config_t bc = {
        .i2c_port = -1,
        .sda_io_num = cfg->gpio_sda,
        .scl_io_num = cfg->gpio_scl,
        .clk_source = I2C_CLK_SRC_DEFAULT,
        .glitch_ignore_cnt = 7,
        .flags.enable_internal_pullup = true, // the STEMMA QT boards have their own as well
    };
    ESP_RETURN_ON_ERROR(i2c_new_master_bus(&bc, &p.bus), TAG, "i2c bus");
    if (i2c_master_probe(p.bus, MAX17048_ADDR, I2C_TIMEOUT_MS) != ESP_OK) {
        ESP_LOGW(TAG, "no MAX17048 fuel gauge – battery unknown (USB build?)");
        return ESP_OK;
    }
    i2c_device_config_t dc = {.dev_addr_length = I2C_ADDR_BIT_LEN_7, .device_address = MAX17048_ADDR, .scl_speed_hz = 100000};
    ESP_RETURN_ON_ERROR(i2c_master_bus_add_device(p.bus, &dc, &p.gauge), TAG, "gauge");
    ESP_LOGI(TAG, "MAX17048 fuel gauge found");
    return ESP_OK;
}

static esp_err_t read_reg(uint8_t reg, uint16_t *v)
{
    uint8_t b[2];
    esp_err_t err = i2c_master_transmit_receive(p.gauge, &reg, 1, b, 2, I2C_TIMEOUT_MS);
    if (err == ESP_OK) {
        *v = (uint16_t)(b[0] << 8 | b[1]);
    }
    return err;
}

void dm_power_read(dm_power_t *out)
{
    *out = (dm_power_t){0};
    if (p.gauge != NULL) {
        uint16_t vcell, soc;
        if (read_reg(REG_VCELL, &vcell) == ESP_OK && read_reg(REG_SOC, &soc) == ESP_OK) {
            out->gauge = true;
            out->cell_mv = (int)((uint32_t)vcell * 78125 / 1000000);
            out->soc_pct = soc / 256.0f;
            if (out->soc_pct > 100.0f) {
                out->soc_pct = 100.0f; // the gauge reports a bit above 100 % when full
            }
        }
    }
    out->charging = p.cfg.gpio_chg >= 0 && gpio_get_level(p.cfg.gpio_chg) == 0;
    // Without a PGOOD pin and without a gauge, it can only be a USB build.
    out->usb = p.cfg.gpio_pgood >= 0 ? gpio_get_level(p.cfg.gpio_pgood) == 0 : !out->gauge;
}
