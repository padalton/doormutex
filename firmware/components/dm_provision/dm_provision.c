#include "dm_provision.h"

#include <stdio.h>
#include <string.h>

#include "dm_improv.h"
#include "dm_net.h"
#include "driver/usb_serial_jtag.h"
#include "driver/usb_serial_jtag_vfs.h"
#include "esp_check.h"
#include "esp_log.h"
#include "esp_system.h"
#include "esp_wifi.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "nvs.h"

static const char *TAG = "setup";
static const char *NS = "doormutex";

#define WIFI_TIMEOUT_MS 30000
#define MAX_APS 16

esp_err_t dm_settings_load(dm_config_t *c)
{
    dm_config_defaults(c);
    nvs_handle_t h;
    esp_err_t err = nvs_open(NS, NVS_READONLY, &h);
    if (err == ESP_ERR_NVS_NOT_FOUND) {
        return ESP_OK; // never configured
    }
    ESP_RETURN_ON_ERROR(err, TAG, "nvs open");
    for (size_t i = 0; i < dm_config_key_count; i++) {
        char v[128];
        size_t len = sizeof(v);
        // NVS keys are at most 15 characters – all of ours fit.
        if (nvs_get_str(h, dm_config_keys[i], v, &len) == ESP_OK &&
            dm_config_set(c, dm_config_keys[i], v) != DM_CONFIG_OK) {
            ESP_LOGW(TAG, "ignoring stored %s", dm_config_keys[i]);
        }
    }
    nvs_close(h);
    return ESP_OK;
}

esp_err_t dm_settings_save(const dm_config_t *c)
{
    nvs_handle_t h;
    ESP_RETURN_ON_ERROR(nvs_open(NS, NVS_READWRITE, &h), TAG, "nvs open");
    esp_err_t err = ESP_OK;
    for (size_t i = 0; i < dm_config_key_count && err == ESP_OK; i++) {
        char v[128];
        dm_config_get(c, dm_config_keys[i], false, v, sizeof(v));
        err = nvs_set_str(h, dm_config_keys[i], v);
    }
    if (err == ESP_OK) {
        err = nvs_commit(h);
    }
    nvs_close(h);
    return err;
}

esp_err_t dm_settings_erase(void)
{
    nvs_handle_t h;
    ESP_RETURN_ON_ERROR(nvs_open(NS, NVS_READWRITE, &h), TAG, "nvs open");
    esp_err_t err = nvs_erase_all(h);
    if (err == ESP_OK) {
        err = nvs_commit(h);
    }
    nvs_close(h);
    return err;
}

// ---- serial: Improv frames and console lines share the USB connection

static struct {
    dm_config_t *cfg;
    dm_serial_cfg_t sc;
    dm_improv_parser_t parser;
    char line[192];
    size_t line_len;
} s;

static void out_bytes(const void *b, size_t n)
{
    usb_serial_jtag_write_bytes(b, n, pdMS_TO_TICKS(100));
}

static void out(const char *text)
{
    out_bytes(text, strlen(text));
}

static void improv_state(dm_improv_state_t st)
{
    uint8_t f[16];
    out_bytes(f, dm_improv_build_state(st, f, sizeof(f)));
}

static void improv_error(dm_improv_error_t e)
{
    uint8_t f[16];
    out_bytes(f, dm_improv_build_error(e, f, sizeof(f)));
}

static void improv_result(uint8_t cmd, const char *const *strs, size_t n)
{
    uint8_t f[300];
    size_t len = dm_improv_build_result(cmd, strs, n, f, sizeof(f));
    if (len > 0) {
        out_bytes(f, len);
    }
}

static dm_improv_state_t current_improv_state(void)
{
    return dm_net_has_credentials() && dm_net_rssi() != 0 ? DM_IMPROV_STATE_PROVISIONED : DM_IMPROV_STATE_READY;
}

static void handle_rpc(const dm_improv_frame_t *frame)
{
    dm_improv_rpc_t rpc;
    if (dm_improv_parse_rpc(frame, &rpc) != 0) {
        improv_error(DM_IMPROV_ERROR_INVALID_RPC);
        return;
    }
    switch (rpc.command) {
    case DM_IMPROV_CMD_WIFI_SETTINGS:
        improv_error(DM_IMPROV_ERROR_NONE);
        improv_state(DM_IMPROV_STATE_PROVISIONING);
        ESP_LOGI(TAG, "Improv: connecting to \"%s\"", rpc.ssid);
        if (dm_net_try_wifi(rpc.ssid, rpc.password, WIFI_TIMEOUT_MS) == ESP_OK) {
            improv_state(DM_IMPROV_STATE_PROVISIONED);
            // No device web page yet, so no "next URL" for ESP Web Tools.
            improv_result(DM_IMPROV_CMD_WIFI_SETTINGS, NULL, 0);
        } else {
            improv_error(DM_IMPROV_ERROR_UNABLE_TO_CONNECT);
            improv_state(DM_IMPROV_STATE_READY);
        }
        memset(&rpc, 0, sizeof(rpc)); // don't keep the password around
        break;
    case DM_IMPROV_CMD_GET_CURRENT_STATE: {
        dm_improv_state_t st = current_improv_state();
        improv_state(st);
        if (st == DM_IMPROV_STATE_PROVISIONED) {
            improv_result(DM_IMPROV_CMD_GET_CURRENT_STATE, NULL, 0);
        }
        break;
    }
    case DM_IMPROV_CMD_GET_DEVICE_INFO: {
        const char *info[] = {"doormutex", s.sc.firmware, "ESP32-C6", s.sc.device_id};
        improv_result(DM_IMPROV_CMD_GET_DEVICE_INFO, info, 4);
        break;
    }
    case DM_IMPROV_CMD_GET_WIFI_NETWORKS: {
        static dm_net_ap_t aps[MAX_APS];
        int n = dm_net_scan(aps, MAX_APS);
        for (int i = 0; i < n; i++) {
            char rssi[8];
            snprintf(rssi, sizeof(rssi), "%d", aps[i].rssi);
            const char *row[] = {aps[i].ssid, rssi, aps[i].secured ? "YES" : "NO"};
            improv_result(DM_IMPROV_CMD_GET_WIFI_NETWORKS, row, 3);
        }
        improv_result(DM_IMPROV_CMD_GET_WIFI_NETWORKS, NULL, 0); // end of list
        break;
    }
    default:
        improv_error(DM_IMPROV_ERROR_UNKNOWN_RPC);
    }
}

static void print_settings(void)
{
    char buf[200], v[128];
    snprintf(buf, sizeof(buf), "device     %s\nfirmware   %s\nwifi       %s\n", s.sc.device_id, s.sc.firmware,
             dm_net_has_credentials() ? (dm_net_rssi() != 0 ? "connected" : "configured, not connected")
                                      : "not configured (use ESP Web Tools / Improv)");
    out(buf);
    for (size_t i = 0; i < dm_config_key_count; i++) {
        dm_config_get(s.cfg, dm_config_keys[i], true, v, sizeof(v));
        snprintf(buf, sizeof(buf), "%-16s %s\n", dm_config_keys[i], v);
        out(buf);
    }
}

static void handle_line(const char *line)
{
    dm_cmd_t cmd;
    dm_console_parse(line, &cmd);
    switch (cmd.kind) {
    case DM_CMD_NONE:
        return;
    case DM_CMD_HELP:
        out("commands:\n"
            "  show                 settings and status\n"
            "  set KEY VALUE        change a setting (no value: default), then restart\n"
            "  restart              reboot\n"
            "  identify             cycle the lamps\n"
            "  factory-reset        forget settings and Wi-Fi\n"
            "keys: mqtt_url mqtt_user mqtt_pass profile base_topic discovery_prefix ntp brightness\n");
        return;
    case DM_CMD_SHOW:
        print_settings();
        return;
    case DM_CMD_SET: {
        int r = dm_config_set(s.cfg, cmd.key, cmd.value);
        if (r == DM_CONFIG_UNKNOWN_KEY) {
            out("unknown key – try `help`\n");
        } else if (r == DM_CONFIG_INVALID) {
            out("invalid value\n");
        } else if (dm_settings_save(s.cfg) != ESP_OK) {
            out("could not save\n");
        } else {
            out("saved – `restart` to apply\n");
        }
        memset(&cmd, 0, sizeof(cmd));
        return;
    }
    case DM_CMD_RESTART:
        out("restarting\n");
        vTaskDelay(pdMS_TO_TICKS(100));
        esp_restart();
        return;
    case DM_CMD_FACTORY_RESET:
        out("erasing settings and Wi-Fi, restarting\n");
        dm_settings_erase();
        esp_wifi_restore();
        vTaskDelay(pdMS_TO_TICKS(100));
        esp_restart();
        return;
    case DM_CMD_IDENTIFY:
        if (s.sc.on_identify) {
            s.sc.on_identify();
        }
        return;
    case DM_CMD_UNKNOWN:
        out("unknown command – try `help`\n");
        return;
    }
}

static void text_byte(char c)
{
    if (c == '\n' || c == '\r') {
        if (s.line_len > 0) {
            s.line[s.line_len] = '\0';
            handle_line(s.line);
            memset(s.line, 0, sizeof(s.line)); // may have held a password
        }
        s.line_len = 0;
    } else if (s.line_len < sizeof(s.line) - 1) {
        s.line[s.line_len++] = c;
    }
}

static void serial_task(void *arg)
{
    (void)arg;
    uint8_t buf[64];
    dm_improv_frame_t frame;
    for (;;) {
        int n = usb_serial_jtag_read_bytes(buf, sizeof(buf), portMAX_DELAY);
        for (int i = 0; i < n; i++) {
            dm_improv_feed_t r = dm_improv_feed(&s.parser, buf[i], &frame);
            switch (r.result) {
            case DM_IMPROV_FRAME:
                if (frame.type == DM_IMPROV_TYPE_RPC) {
                    handle_rpc(&frame);
                }
                break;
            case DM_IMPROV_TEXT:
                for (uint8_t k = 0; k < r.text_prefix; k++) {
                    text_byte("IMPROV"[k]);
                }
                if (r.byte_is_text) {
                    text_byte((char)buf[i]);
                }
                break;
            case DM_IMPROV_BAD_CHECKSUM:
                improv_error(DM_IMPROV_ERROR_INVALID_RPC);
                break;
            case DM_IMPROV_MORE:
                break;
            }
        }
    }
}

esp_err_t dm_serial_start(dm_config_t *cfg, const dm_serial_cfg_t *sc)
{
    s.cfg = cfg;
    s.sc = *sc;
    dm_improv_init(&s.parser);
    usb_serial_jtag_driver_config_t dc = USB_SERIAL_JTAG_DRIVER_CONFIG_DEFAULT();
    ESP_RETURN_ON_ERROR(usb_serial_jtag_driver_install(&dc), TAG, "usb serial driver");
    // Logs and printf now go through the driver too, so they don't fight
    // with Improv frames for the hardware FIFO.
    usb_serial_jtag_vfs_use_driver();
    if (xTaskCreate(serial_task, "serial", 4096, NULL, 5, NULL) != pdPASS) {
        return ESP_ERR_NO_MEM;
    }
    return ESP_OK;
}
