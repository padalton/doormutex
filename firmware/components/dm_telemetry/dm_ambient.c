// VEML7700 ambient light sensor (I²C 0x10), written from Vishay's datasheet
// and application note: 100 ms integration, gain ×2 for dim rooms and ×1/8
// when that saturates, power-saving mode between measurements.

#include "dm_telemetry.h"
#include "dm_telemetry_priv.h"

#include <stdbool.h>
#include <stdio.h>

#include "driver/i2c_master.h"
#include "esp_check.h"
#include "esp_log.h"

static const char *TAG = "ambient";

#define VEML7700_ADDR 0x10
#define REG_CONF 0x00
#define REG_PSM 0x03
#define REG_ALS 0x04
#define I2C_TIMEOUT_MS 50

// ALS_CONF: gain in bits 12:11, integration time in 9:6 (0000 = 100 ms).
#define CONF_GAIN_2 (0x1 << 11)
#define CONF_GAIN_1_8 (0x2 << 11)
// PSM: mode 4 (a measurement every ~4 s), enabled – a few µA.
#define PSM_MODE4_ON 0x0007
// lx per count at 100 ms (application note, gain ×2 → ×1/8 is 16 times).
#define RES_GAIN_2 0.0336f
#define RES_GAIN_1_8 0.5376f

static struct {
    char status[64];
    i2c_master_dev_handle_t dev;
    bool low_gain;
    bool skip; // the first value after a gain change is still the old one
} a;

static esp_err_t write_reg(uint8_t reg, uint16_t v)
{
    uint8_t b[3] = {reg, (uint8_t)(v & 0xFF), (uint8_t)(v >> 8)}; // LSB first
    return i2c_master_transmit(a.dev, b, sizeof(b), I2C_TIMEOUT_MS);
}

static esp_err_t read_reg(uint8_t reg, uint16_t *v)
{
    uint8_t b[2];
    esp_err_t err = i2c_master_transmit_receive(a.dev, &reg, 1, b, 2, I2C_TIMEOUT_MS);
    if (err == ESP_OK) {
        *v = (uint16_t)(b[1] << 8 | b[0]);
    }
    return err;
}

static esp_err_t set_gain(bool low)
{
    a.low_gain = low;
    a.skip = true;
    return write_reg(REG_CONF, low ? CONF_GAIN_1_8 : CONF_GAIN_2);
}

esp_err_t dm_ambient_init(void)
{
    i2c_master_bus_handle_t bus = dm_power_i2c_bus();
    if (bus == NULL) {
        return ESP_ERR_INVALID_STATE;
    }
    if (i2c_master_probe(bus, VEML7700_ADDR, I2C_TIMEOUT_MS) != ESP_OK) {
        // Say what is on the bus instead – tells a swapped SDA/SCL (nothing)
        // from a different sensor or address.
        int n = snprintf(a.status, sizeof(a.status), "no sensor (I2C:");
        int found = 0;
        for (uint8_t addr = 0x08; addr < 0x78 && n < (int)sizeof(a.status) - 6; addr++) {
            if (i2c_master_probe(bus, addr, 10) == ESP_OK) {
                n += snprintf(a.status + n, sizeof(a.status) - n, " 0x%02x", addr);
                found++;
            }
        }
        snprintf(a.status + n, sizeof(a.status) - n, "%s)", found ? "" : " nothing");
        ESP_LOGW(TAG, "no VEML7700 light sensor – fixed brightness; %s", a.status);
        return ESP_ERR_NOT_FOUND;
    }
    i2c_device_config_t dc = {.dev_addr_length = I2C_ADDR_BIT_LEN_7, .device_address = VEML7700_ADDR, .scl_speed_hz = 100000};
    ESP_RETURN_ON_ERROR(i2c_master_bus_add_device(bus, &dc, &a.dev), TAG, "device");
    ESP_RETURN_ON_ERROR(write_reg(REG_PSM, PSM_MODE4_ON), TAG, "psm");
    ESP_RETURN_ON_ERROR(set_gain(false), TAG, "conf"); // also leaves shutdown
    snprintf(a.status, sizeof(a.status), "VEML7700");
    ESP_LOGI(TAG, "VEML7700 light sensor found");
    return ESP_OK;
}

const char *dm_ambient_status(void)
{
    return a.status[0] ? a.status : "no I2C bus";
}

bool dm_ambient_lux(float *lux)
{
    if (a.dev == NULL) {
        return false;
    }
    uint16_t raw;
    if (read_reg(REG_ALS, &raw) != ESP_OK) {
        return false;
    }
    if (a.skip) {
        a.skip = false;
        return false;
    }
    // Switch range with hysteresis: ×2 saturates at ~2200 lx.
    if (!a.low_gain && raw > 50000) {
        set_gain(true);
        return false;
    }
    if (a.low_gain && raw < 2000) {
        set_gain(false);
        return false;
    }
    *lux = (float)raw * (a.low_gain ? RES_GAIN_1_8 : RES_GAIN_2);
    return true;
}
