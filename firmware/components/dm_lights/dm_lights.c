#include "dm_lights.h"

#include <stdint.h>

#include "dm_pattern.h"
#include "driver/ledc.h"
#include "esp_check.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"

static const char *TAG = "lights";

// RC_FAST (~17.5 MHz) keeps running in light sleep, so the PWM does too
// (LEDC_SLEEP_MODE_KEEP_ALIVE). 1 kHz is flicker-free; 12 bits leave room
// for deep dimming at night.
#define PWM_FREQ_HZ 1000
#define PWM_RES LEDC_TIMER_12_BIT
#define PWM_MAX ((1u << 12) - 1)
#define IDENTIFY_STEP_MS 400
#define IDENTIFY_STEPS 9

static const uint8_t lamp_bits[3] = {DM_LAMP_RED, DM_LAMP_YELLOW, DM_LAMP_GREEN};

static struct {
    dm_lights_cfg_t cfg;
    SemaphoreHandle_t mu;
    esp_timer_handle_t timer;
    dm_light_t light;
    int64_t started_us; // when the current pattern started
    float brightness;
    int supply_mv;
    int identify_left;
} s = {.brightness = 0.4f};

static void apply_mask(uint8_t mask)
{
    const int vf[3] = {s.cfg.vf_mv_red, s.cfg.vf_mv_yellow, s.cfg.vf_mv_green};
    for (int ch = 0; ch < 3; ch++) {
        uint32_t duty = (mask & lamp_bits[ch]) ? dm_led_duty(s.brightness, s.supply_mv, vf[ch], s.cfg.ref_mv, PWM_MAX) : 0;
        ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t)ch, duty);
        ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t)ch);
    }
}

// render shows the pattern's current step and arms the timer for the next
// one. Steady patterns arm nothing – the CPU can sleep, LEDC keeps lighting.
static void render_locked(void)
{
    esp_timer_stop(s.timer); // fine if it isn't running
    if (s.identify_left > 0) {
        apply_mask(lamp_bits[(IDENTIFY_STEPS - s.identify_left) % 3]);
        s.identify_left--;
        esp_timer_start_once(s.timer, IDENTIFY_STEP_MS * 1000);
        if (s.identify_left == 0) {
            s.started_us = esp_timer_get_time() + IDENTIFY_STEP_MS * 1000;
        }
        return;
    }
    uint32_t next;
    uint32_t elapsed = (uint32_t)((esp_timer_get_time() - s.started_us) / 1000);
    apply_mask(dm_pattern_at(dm_pattern_for(s.light), elapsed, &next));
    if (next != UINT32_MAX) {
        esp_timer_start_once(s.timer, (uint64_t)next * 1000);
    }
}

static void on_timer(void *arg)
{
    (void)arg;
    xSemaphoreTake(s.mu, portMAX_DELAY);
    render_locked();
    xSemaphoreGive(s.mu);
}

esp_err_t dm_lights_init(const dm_lights_cfg_t *cfg)
{
    s.cfg = *cfg;
    s.mu = xSemaphoreCreateMutex();
    s.light = DM_LIGHT_OFF;
    s.started_us = esp_timer_get_time();

    ledc_timer_config_t timer = {
        .speed_mode = LEDC_LOW_SPEED_MODE,
        .duty_resolution = PWM_RES,
        .timer_num = LEDC_TIMER_0,
        .freq_hz = PWM_FREQ_HZ,
        .clk_cfg = LEDC_USE_RC_FAST_CLK,
    };
    ESP_RETURN_ON_ERROR(ledc_timer_config(&timer), TAG, "timer");
    const int gpios[3] = {cfg->gpio_red, cfg->gpio_yellow, cfg->gpio_green};
    for (int ch = 0; ch < 3; ch++) {
        ledc_channel_config_t c = {
            .gpio_num = gpios[ch],
            .speed_mode = LEDC_LOW_SPEED_MODE,
            .channel = (ledc_channel_t)ch,
            .timer_sel = LEDC_TIMER_0,
            .duty = 0,
            // Keep the PWM running while the CPU is in light sleep – the
            // whole point of an always-on light on a battery.
            // TODO(breadboard): verify the RC_FAST clock stays on in light
            // sleep on the C6 and measure its current.
            .sleep_mode = LEDC_SLEEP_MODE_KEEP_ALIVE,
        };
        ESP_RETURN_ON_ERROR(ledc_channel_config(&c), TAG, "channel %d", ch);
    }
    const esp_timer_create_args_t args = {.callback = on_timer, .name = "lights"};
    ESP_RETURN_ON_ERROR(esp_timer_create(&args, &s.timer), TAG, "esp_timer");
    ESP_LOGI(TAG, "LEDs on GPIO %d/%d/%d", cfg->gpio_red, cfg->gpio_yellow, cfg->gpio_green);
    return ESP_OK;
}

void dm_lights_show(dm_light_t light)
{
    xSemaphoreTake(s.mu, portMAX_DELAY);
    if (light != s.light) {
        ESP_LOGI(TAG, "%s → %s", dm_light_name(s.light), dm_light_name(light));
        s.light = light;
        s.started_us = esp_timer_get_time();
        render_locked();
    }
    xSemaphoreGive(s.mu);
}

void dm_lights_set_brightness(float brightness)
{
    xSemaphoreTake(s.mu, portMAX_DELAY);
    s.brightness = brightness;
    render_locked();
    xSemaphoreGive(s.mu);
}

void dm_lights_set_supply_mv(int mv)
{
    xSemaphoreTake(s.mu, portMAX_DELAY);
    s.supply_mv = mv;
    render_locked();
    xSemaphoreGive(s.mu);
}

void dm_lights_identify(void)
{
    xSemaphoreTake(s.mu, portMAX_DELAY);
    s.identify_left = IDENTIFY_STEPS;
    render_locked();
    xSemaphoreGive(s.mu);
}

dm_light_t dm_lights_current(void)
{
    return s.light;
}
