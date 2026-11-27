// doormutex traffic light – firmware entry point.
//
// Smart hub, dumb light: the hub publishes the state (incl. the lamp
// pattern) via MQTT; the light shows it, reports battery and signal, and
// blinks yellow by itself when it can't trust the hub any more.
#include <math.h>
#include <stdio.h>
#include <string.h>
#include <time.h>

#include "dm_config.h"
#include "dm_discovery.h"
#include "dm_eink.h"
#include "dm_lights.h"
#include "dm_net.h"
#include "dm_pattern.h"
#include "dm_provision.h"
#include "dm_state.h"
#include "dm_telemetry.h"
#include "esp_app_desc.h"
#include "esp_attr.h"
#include "esp_log.h"
#include "esp_mac.h"
#include "esp_ota_ops.h"
#include "esp_pm.h"
#include "esp_system.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "freertos/task.h"
#include "nvs_flash.h"

static const char *TAG = "doormutex";

static dm_config_t cfg;
static dm_device_t dev;
static dm_topics_t topics;
static TaskHandle_t main_task;

// Shared between the MQTT task (callbacks) and the main loop.
static struct {
    SemaphoreHandle_t mu;
    dm_state_t state;
    bool have_state;
    bool hub_online_known, hub_online;
    bool connected;
    int64_t disconnected_ms;
    bool announce; // (re)connected: publish discovery and telemetry
    bool app_confirmed;
} sh;

// ---- connection watchdog note
//
// Survives esp_restart() (not a power cut): what the watchdog saw when it
// restarted the chip, and how many restarts in a row it has done.
#define NOTE_MAGIC 0x646d7764u // "dmwd"
typedef struct {
    uint32_t magic;
    dm_outage_t outage;
} watchdog_note_t;
static RTC_NOINIT_ATTR watchdog_note_t note;

static const char *reset_reason = "unknown";
static const dm_outage_t *last_outage; // set after a watchdog restart
static uint32_t watchdog_restarts;     // in a row; 0 again once connected

static const char *reset_name(esp_reset_reason_t r, bool watchdog)
{
    if (watchdog) {
        return "net_watchdog";
    }
    switch (r) {
    case ESP_RST_POWERON:
        return "power_on";
    case ESP_RST_SW:
        return "software"; // restart command, console, after flashing
    case ESP_RST_PANIC:
        return "panic";
    case ESP_RST_INT_WDT:
    case ESP_RST_TASK_WDT:
    case ESP_RST_WDT:
        return "wdt";
    case ESP_RST_BROWNOUT:
        return "brownout";
    case ESP_RST_DEEPSLEEP:
        return "deep_sleep";
    case ESP_RST_USB:
        return "usb"; // e.g. a serial terminal toggling DTR/RTS
    case ESP_RST_JTAG:
        return "jtag";
    case ESP_RST_EXT:
        return "external";
    default:
        return "other";
    }
}

static int64_t now_ms(void)
{
    return esp_timer_get_time() / 1000;
}

static void wake_main(void)
{
    if (main_task) {
        xTaskNotifyGive(main_task);
    }
}

// ---- MQTT callbacks (run in the MQTT task)

static void on_state(const char *json, size_t len)
{
    static dm_state_t parsed; // static: ~400 bytes, keep them off the MQTT task's stack
    int err = dm_state_parse(json, len, &parsed);
    if (err != DM_STATE_OK) {
        ESP_LOGW(TAG, "state ignored (%d)", err);
        return;
    }
    xSemaphoreTake(sh.mu, portMAX_DELAY);
    sh.state = parsed;
    sh.have_state = true;
    bool confirm = !sh.app_confirmed;
    sh.app_confirmed = true;
    xSemaphoreGive(sh.mu);
    if (confirm) {
        // A state from the hub proves this firmware works end to end: keep
        // it after an OTA update instead of rolling back.
        esp_ota_mark_app_valid_cancel_rollback();
    }
    wake_main();
}

static void on_hub_online(bool online)
{
    xSemaphoreTake(sh.mu, portMAX_DELAY);
    sh.hub_online_known = true;
    sh.hub_online = online;
    xSemaphoreGive(sh.mu);
    ESP_LOGI(TAG, "hub %s", online ? "online" : "offline");
    wake_main();
}

static void on_display(const uint8_t *data, size_t len)
{
    dm_eink_show(data, len);
}

static void on_cmd(const char *cmd, size_t len)
{
    if (len == 8 && memcmp(cmd, "identify", 8) == 0) {
        dm_lights_identify();
    } else if (len == 7 && memcmp(cmd, "restart", 7) == 0) {
        ESP_LOGI(TAG, "restart requested");
        esp_restart();
    } else if (len > 4 && memcmp(cmd, "ota ", 4) == 0) {
        // TODO(firmware): esp_https_ota from the GitHub release URL, then
        // restart; the rollback above protects against a broken image.
        ESP_LOGW(TAG, "OTA is not implemented yet");
    } else {
        ESP_LOGW(TAG, "unknown command %.*s", (int)len, cmd);
    }
}

static void on_link(bool up)
{
    xSemaphoreTake(sh.mu, portMAX_DELAY);
    if (up && !sh.connected) {
        sh.announce = true;
    }
    if (!up && sh.connected) {
        sh.disconnected_ms = now_ms();
    }
    sh.connected = up;
    xSemaphoreGive(sh.mu);
    wake_main();
}

// Live values for the console's `show`, written by the main loop.
static struct {
    float lux;
    float brightness;
} live = {.lux = -1.0f};

static void status(char *out, size_t cap)
{
    dm_power_t pw;
    dm_power_read(&pw);
    char lux[72];
    snprintf(lux, sizeof(lux), "%s", dm_ambient_status());
    if (live.lux >= 0.0f) {
        snprintf(lux, sizeof(lux), "%.1f lx", (double)live.lux);
    }
    char bat[32] = "no gauge";
    if (pw.gauge) {
        snprintf(bat, sizeof(bat), "%.0f %% (%d mV)", (double)pw.soc_pct, pw.cell_mv);
    }
    char restart[160];
    snprintf(restart, sizeof(restart), "%s", reset_reason);
    if (last_outage) {
        const dm_outage_t *o = last_outage;
        snprintf(restart, sizeof(restart), "%s after %lu s offline (Wi-Fi %s, IP %s, MQTT %s, reason %d, errno %d, %s)",
                 reset_reason, (unsigned long)o->down_s, o->wifi ? "up" : "down", o->ip ? "yes" : "no",
                 o->mqtt ? "up" : "down", o->wifi_reason, o->sock_errno,
                 o->mqtt_error[0] ? o->mqtt_error : "no MQTT error");
    }
    snprintf(out, cap,
             "light      %s\nambient    %s -> brightness %.0f %%\nbattery    %s%s%s\neink       %s\nrestart    %s\n",
             dm_light_name(dm_lights_current()), lux, (double)(live.brightness * 100.0f), bat, pw.usb ? ", USB" : "",
             pw.charging ? ", charging" : "", dm_eink_status(), restart);
}

// ---- main loop

static void publish_telemetry(const char *light, const char *fault, const dm_power_t *pw, float lux)
{
    dm_telemetry_t t = {
        .battery_pct = pw->gauge ? pw->soc_pct : -1.0f,
        .battery_mv = pw->gauge ? pw->cell_mv : 0,
        .charging = pw->charging,
        .usb = pw->usb,
        .rssi = dm_net_rssi(),
        .has_lux = lux >= 0.0f,
        .lux = lux,
        .light = light,
        .fault = fault,
        .uptime_s = (uint32_t)(now_ms() / 1000),
        .firmware = dev.firmware,
        .reset = reset_reason,
        .outage = last_outage,
        .profile = dev.profile,
        .eink_battery = dm_eink_battery(),
    };
    char doc[640];
    size_t n = dm_telemetry_json(&t, doc, sizeof(doc));
    if (n > 0) {
        dm_net_publish(topics.telemetry, doc, n, true);
    }
}

static void publish_discovery(void)
{
    static char payload[4096]; // ~3.4 kB today; test_discovery checks it fits
    char topic[128];
    size_t n = dm_discovery(&dev, topic, sizeof(topic), payload, sizeof(payload));
    if (n == 0) {
        ESP_LOGE(TAG, "discovery payload too large");
        return;
    }
    dm_net_publish(topic, payload, n, true);
}

// watchdog_restart notes what the network layer saw and restarts the chip.
static void watchdog_restart(int64_t down_ms)
{
    dm_net_diag_t d;
    dm_net_diag(&d);
    note.outage = (dm_outage_t){
        .down_s = (uint32_t)(down_ms / 1000),
        .restarts = watchdog_restarts + 1,
        .wifi = d.associated,
        .ip = d.ip,
        .mqtt = d.mqtt,
        .wifi_reason = d.wifi_reason,
        .sock_errno = d.sock_errno,
    };
    strlcpy(note.outage.mqtt_error, d.mqtt_error, sizeof(note.outage.mqtt_error));
    note.magic = NOTE_MAGIC;
    ESP_LOGE(TAG, "offline for %lld s (Wi-Fi %s, IP %s, MQTT %s, reason %d, errno %d, %s) – restart %lu",
             (long long)(down_ms / 1000), d.associated ? "up" : "down", d.ip ? "yes" : "no", d.mqtt ? "up" : "down",
             d.wifi_reason, d.sock_errno, d.mqtt_error[0] ? d.mqtt_error : "no MQTT error",
             (unsigned long)note.outage.restarts);
    esp_restart();
}

static void run(bool net_started)
{
    const dm_failsafe_cfg_t fs = {
        // After a watchdog restart the light was blinking yellow already:
        // keep blinking instead of going dark for the boot grace period.
        .boot_grace_ms = last_outage ? 0 : CONFIG_DM_BOOT_GRACE_S * 1000,
        .stale_after_ms = CONFIG_DM_STALE_AFTER_S * 1000,
        .until_grace_s = CONFIG_DM_UNTIL_GRACE_S,
    };
    const dm_netwatch_cfg_t nw = {
        .kick_after_ms = CONFIG_DM_NET_KICK_AFTER_S * 1000,
        .restart_after_ms = CONFIG_DM_NET_RESTART_AFTER_S * 1000,
        .restart_max_ms = CONFIG_DM_NET_RESTART_MAX_S * 1000,
    };
    int64_t last_kick = -1;
    const int64_t boot = now_ms();
    int64_t next_telemetry = 0;
    dm_fault_t last_fault = DM_FAULT_BOOTING;
    dm_power_t last_power = {0};
    int last_eink_battery = -1;
    float lux = -1.0f; // smoothed ambient light, < 0 = no sensor (yet)
    float brightness = cfg.brightness / 100.0f;
    static dm_state_t st;

    for (;;) {
        // Wake on news from MQTT, otherwise once a second for the timers.
        ulTaskNotifyTake(pdTRUE, pdMS_TO_TICKS(1000));

        dm_link_t link = {.now_ms = now_ms(), .boot_ms = boot};
        bool announce;
        xSemaphoreTake(sh.mu, portMAX_DELAY);
        st = sh.state;
        link.have_state = sh.have_state;
        link.hub_online_known = sh.hub_online_known;
        link.hub_online = sh.hub_online;
        link.connected = sh.connected;
        // Never connected counts as disconnected since boot.
        link.disconnected_ms = sh.disconnected_ms ? sh.disconnected_ms : boot;
        announce = sh.announce;
        sh.announce = false;
        xSemaphoreGive(sh.mu);
        link.clock_valid = dm_net_clock_valid();
        link.now_unix = time(NULL);

        if (link.connected) {
            last_kick = -1;
            watchdog_restarts = 0;
        } else if (net_started) {
            // Without credentials or a broker there's nothing to fix.
            int64_t down = link.now_ms - link.disconnected_ms;
            switch (dm_netwatch(&nw, down, last_kick < 0 ? -1 : link.now_ms - last_kick, watchdog_restarts)) {
            case DM_NETWATCH_KICK:
                dm_net_kick();
                last_kick = link.now_ms;
                break;
            case DM_NETWATCH_RESTART:
                watchdog_restart(down);
                break;
            case DM_NETWATCH_WAIT:
                break;
            }
        }

        dm_fault_t fault = dm_failsafe_check(&fs, &link, link.have_state ? &st : NULL);
        dm_light_t light = dm_effective_light(fault, &st);
        dm_lights_show(light);
        if (fault != last_fault) {
            if (fault != DM_FAULT_NONE) {
                ESP_LOGW(TAG, "failsafe: %s", dm_fault_name(fault));
            }
        }

        dm_power_t pw;
        dm_power_read(&pw);
        if (pw.gauge) {
            dm_lights_set_supply_mv(pw.cell_mv);
        }
        dm_eink_set_battery(pw.gauge ? (int)(pw.soc_pct + 0.5f) : -1, pw.charging);
        float raw;
        if (dm_ambient_lux(&raw)) {
            // Smooth out a passing shadow; the eye adapts slowly as well.
            lux = lux < 0.0f ? raw : lux + 0.2f * (raw - lux);
            float b = dm_brightness_for_lux(cfg.brightness / 100.0f, lux);
            if (fabsf(b - brightness) >= 0.01f) {
                ESP_LOGD(TAG, "%.1f lx → brightness %.0f %%", (double)lux, (double)(b * 100.0f));
                brightness = b;
                dm_lights_set_brightness(b);
            }
        }
        live.lux = lux;
        live.brightness = brightness;
        int eink_battery = dm_eink_battery();
        bool changed = fault != last_fault || pw.charging != last_power.charging || pw.usb != last_power.usb ||
                       eink_battery != last_eink_battery;
        if (announce) {
            publish_discovery();
        }
        if (link.connected && (announce || changed || link.now_ms >= next_telemetry)) {
            publish_telemetry(dm_light_name(light), dm_fault_name(fault), &pw, lux);
            next_telemetry = link.now_ms + CONFIG_DM_TELEMETRY_INTERVAL_S * 1000LL;
        }
        last_fault = fault;
        last_power = pw;
        last_eink_battery = eink_battery;
        // TODO(firmware): away mode (deep sleep + polling while the PC is
        // locked), battery-empty screen and cut-off.
    }
}

static void identify(void)
{
    dm_lights_identify();
}

void app_main(void)
{
    main_task = xTaskGetCurrentTaskHandle();
    sh.mu = xSemaphoreCreateMutex();

    // Read and clear the watchdog's note: RTC memory keeps it across a
    // software restart, but it only counts for the one boot right after.
    static dm_outage_t outage;
    esp_reset_reason_t rr = esp_reset_reason();
    bool by_watchdog = rr == ESP_RST_SW && note.magic == NOTE_MAGIC;
    note.magic = 0;
    if (by_watchdog) {
        outage = note.outage;
        last_outage = &outage;
        watchdog_restarts = outage.restarts;
    }
    reset_reason = reset_name(rr, by_watchdog);

    esp_err_t err = nvs_flash_init();
    if (err == ESP_ERR_NVS_NO_FREE_PAGES || err == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        err = nvs_flash_init();
    }
    ESP_ERROR_CHECK(err);
    ESP_ERROR_CHECK(dm_settings_load(&cfg));

    uint8_t mac[6];
    ESP_ERROR_CHECK(esp_read_mac(mac, ESP_MAC_WIFI_STA));
    dm_device_id(mac, dev.id, sizeof(dev.id));
    dev.profile = cfg.profile;
    dev.base_topic = cfg.base_topic;
    dev.discovery_prefix = cfg.discovery_prefix;
    dev.firmware = esp_app_get_description()->version;
    dev.model = "ESP32-C6 traffic light";
    dm_topics(&dev, &topics);
    ESP_LOGI(TAG, "%s, firmware %s, profile %s", dev.id, dev.firmware, cfg.profile);

    const dm_lights_cfg_t lc = {
        .gpio_red = CONFIG_DM_GPIO_RED,
        .gpio_yellow = CONFIG_DM_GPIO_YELLOW,
        .gpio_green = CONFIG_DM_GPIO_GREEN,
        .vf_mv_red = CONFIG_DM_VF_RED_MV,
        .vf_mv_yellow = CONFIG_DM_VF_YELLOW_MV,
        .vf_mv_green = CONFIG_DM_VF_GREEN_MV,
        .ref_mv = CONFIG_DM_LED_REF_MV,
        .level_red = CONFIG_DM_LED_LEVEL_RED,
        .level_yellow = CONFIG_DM_LED_LEVEL_YELLOW,
        .level_green = CONFIG_DM_LED_LEVEL_GREEN,
    };
    ESP_ERROR_CHECK(dm_lights_init(&lc));
    dm_lights_set_brightness(cfg.brightness / 100.0f);

    const dm_power_cfg_t pc = {
        .gpio_sda = CONFIG_DM_GPIO_I2C_SDA,
        .gpio_scl = CONFIG_DM_GPIO_I2C_SCL,
        .gpio_chg = CONFIG_DM_GPIO_CHG,
        .gpio_pgood = CONFIG_DM_GPIO_PGOOD,
    };
    if (dm_power_init(&pc) != ESP_OK) {
        ESP_LOGW(TAG, "power monitoring unavailable");
    } else {
        dm_ambient_init(); // logs whether a light sensor answered
    }
    const dm_eink_cfg_t ec = {
        .gpio_mosi = CONFIG_DM_GPIO_EINK_MOSI,
        .gpio_sclk = CONFIG_DM_GPIO_EINK_SCLK,
        .gpio_cs = CONFIG_DM_GPIO_EINK_CS,
        .gpio_dc = CONFIG_DM_GPIO_EINK_DC,
        .gpio_rst = CONFIG_DM_GPIO_EINK_RST,
        .gpio_busy = CONFIG_DM_GPIO_EINK_BUSY,
#if CONFIG_DM_EINK_CONNECTOR_TOP
        .rotate_180 = false,
#else
        .rotate_180 = true,
#endif
    };
    dm_eink_init(&ec);

#if CONFIG_PM_ENABLE
    // Automatic light sleep whenever all tasks are idle; Wi-Fi modem sleep
    // keeps the connection, LEDC keeps the lamps lit.
    const esp_pm_config_t pm = {.max_freq_mhz = 160, .min_freq_mhz = 40, .light_sleep_enable = true};
    if (esp_pm_configure(&pm) != ESP_OK) {
        ESP_LOGW(TAG, "power management not available");
    }
#endif

    ESP_ERROR_CHECK(dm_net_init());
    const dm_serial_cfg_t sc = {.device_id = dev.id, .firmware = dev.firmware, .on_identify = identify, .on_lamps = dm_lights_test, .on_status = status};
    ESP_ERROR_CHECK(dm_serial_start(&cfg, &sc));

    bool net_started = false;
    if (!dm_net_has_credentials()) {
        ESP_LOGW(TAG, "no Wi-Fi yet – set it up with ESP Web Tools (Improv), then `set mqtt_url …`");
    } else if (!dm_config_ready(&cfg)) {
        ESP_LOGW(TAG, "no MQTT broker yet – type `set mqtt_url mqtt://host:1883` and `restart`");
    } else {
        const dm_net_callbacks_t cb = {
            .on_state = on_state,
            .on_hub_online = on_hub_online,
            .on_display = on_display,
            .on_cmd = on_cmd,
            .on_link = on_link,
        };
        ESP_ERROR_CHECK(dm_net_start(&cfg, &dev, &cb));
        net_started = true;
    }
    run(net_started);
}
