#include "dm_net.h"

#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "esp_check.h"
#include "esp_crt_bundle.h"
#include "esp_event.h"
#include "esp_log.h"
#include "esp_netif.h"
#include "esp_netif_sntp.h"
#include "esp_timer.h"
#include "esp_wifi.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
#include "mqtt_client.h"

static const char *TAG = "net";

#define BIT_GOT_IP BIT0
// The largest message the light takes: the e-ink bitmap (200×200 / 8 =
// 5000 bytes) with some room for a header.
#define RX_MAX 6144
// Wi-Fi listen interval in beacons while power saving: the radio wakes for
// every third beacon (~300 ms) – a state change still arrives within ~1 s.
#define LISTEN_INTERVAL 3

static struct {
    EventGroupHandle_t events;
    esp_netif_t *netif;
    esp_timer_handle_t retry;
    uint32_t retry_ms;
    bool want_connection;
    bool associated; // with an access point, maybe without an IP yet
    bool wifi_up, mqtt_up;
    int wifi_reason; // last disconnect reason not caused by dm_net_kick
    bool kicked;     // the next disconnect is our own
    int sock_errno;  // last MQTT socket error
    char mqtt_error[40];
    bool clock_valid;
    esp_mqtt_client_handle_t mqtt;
    const dm_config_t *cfg;
    const dm_device_t *dev;
    dm_topics_t topics;
    dm_net_callbacks_t cb;
    // reassembly of chunked MQTT messages
    char rx_topic[96];
    size_t rx_len;
    bool rx_drop;
} n;

static uint8_t rx_buf[RX_MAX];

// The MQTT client starts with the first IP address: started before, its
// first attempt fails ("Host is unreachable") and the next one only comes
// after reconnect_timeout. Both dm_net_start and the IP event may get
// there first; whoever does starts it, once.
static atomic_bool mqtt_created, mqtt_started;

static void start_mqtt(void)
{
    if (atomic_load(&mqtt_created) && !atomic_exchange(&mqtt_started, true)) {
        esp_err_t err = esp_mqtt_client_start(n.mqtt);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "MQTT start: %s", esp_err_to_name(err));
        }
    }
}

static void link_changed(void)
{
    if (n.cb.on_link) {
        n.cb.on_link(n.wifi_up && n.mqtt_up);
    }
}

// schedule_retry calls wifi_connect() again after the back-off: up to 60 s – a
// light at the end of the hallway retries forever, but doesn't drain the
// cell doing it.
static void schedule_retry(void)
{
    esp_timer_stop(n.retry); // a retry already pending is replaced
    esp_timer_start_once(n.retry, (uint64_t)n.retry_ms * 1000);
    n.retry_ms = n.retry_ms < 30000 ? n.retry_ms * 2 : 60000;
}

// wifi_connect starts a connection attempt. A failed attempt ends in a
// DISCONNECTED event, which schedules the next one – but if the call
// itself fails, no event follows, and without a timer of its own the
// light would never try again.
static void wifi_connect(void)
{
    esp_err_t err = esp_wifi_connect();
    if (err != ESP_OK) {
        ESP_LOGW(TAG, "Wi-Fi connect: %s, retry in %lu ms", esp_err_to_name(err), (unsigned long)n.retry_ms);
        schedule_retry();
    }
}

static void retry_cb(void *arg)
{
    (void)arg;
    // A retry scheduled after a failed call may come late: if another
    // attempt got through in the meantime, leave it alone.
    if (n.want_connection && !n.associated) {
        wifi_connect();
    }
}

static void on_wifi(void *arg, esp_event_base_t base, int32_t id, void *data)
{
    (void)arg;
    (void)base;
    switch (id) {
    case WIFI_EVENT_STA_START:
        if (n.want_connection) {
            wifi_connect();
        }
        break;
    case WIFI_EVENT_STA_CONNECTED:
        n.associated = true;
        n.retry_ms = 1000;
        break;
    case WIFI_EVENT_STA_DISCONNECTED: {
        const wifi_event_sta_disconnected_t *d = data;
        ESP_LOGW(TAG, "Wi-Fi lost (reason %d), retry in %lu ms", d->reason, (unsigned long)n.retry_ms);
        xEventGroupClearBits(n.events, BIT_GOT_IP);
        n.associated = false;
        n.wifi_up = false;
        if (!n.kicked) {
            n.wifi_reason = d->reason;
        }
        n.kicked = false;
        link_changed();
        if (n.want_connection) {
            schedule_retry();
        }
        break;
    }
    default:
        break;
    }
}

static void on_ip(void *arg, esp_event_base_t base, int32_t id, void *data)
{
    (void)arg;
    (void)base;
    (void)id;
    const ip_event_got_ip_t *e = data;
    ESP_LOGI(TAG, "IP " IPSTR, IP2STR(&e->ip_info.ip));
    n.wifi_up = true;
    xEventGroupSetBits(n.events, BIT_GOT_IP);
    link_changed();
    start_mqtt();
}

esp_err_t dm_net_init(void)
{
    n.events = xEventGroupCreate();
    n.retry_ms = 1000;
    ESP_RETURN_ON_ERROR(esp_netif_init(), TAG, "netif");
    ESP_RETURN_ON_ERROR(esp_event_loop_create_default(), TAG, "event loop");
    n.netif = esp_netif_create_default_wifi_sta();
    wifi_init_config_t init = WIFI_INIT_CONFIG_DEFAULT();
    ESP_RETURN_ON_ERROR(esp_wifi_init(&init), TAG, "wifi init");
    ESP_RETURN_ON_ERROR(esp_wifi_set_storage(WIFI_STORAGE_FLASH), TAG, "storage");
    ESP_RETURN_ON_ERROR(esp_event_handler_register(WIFI_EVENT, ESP_EVENT_ANY_ID, on_wifi, NULL), TAG, "wifi events");
    ESP_RETURN_ON_ERROR(esp_event_handler_register(IP_EVENT, IP_EVENT_STA_GOT_IP, on_ip, NULL), TAG, "ip events");
    const esp_timer_create_args_t args = {.callback = retry_cb, .name = "wifi_retry"};
    ESP_RETURN_ON_ERROR(esp_timer_create(&args, &n.retry), TAG, "retry timer");
    ESP_RETURN_ON_ERROR(esp_wifi_set_mode(WIFI_MODE_STA), TAG, "mode");
    ESP_RETURN_ON_ERROR(esp_wifi_start(), TAG, "start");
    return ESP_OK;
}

bool dm_net_has_credentials(void)
{
    wifi_config_t c = {0};
    return esp_wifi_get_config(WIFI_IF_STA, &c) == ESP_OK && c.sta.ssid[0] != '\0';
}

static void configure_sta(const char *ssid, const char *password)
{
    wifi_config_t c = {0};
    if (ssid != NULL) {
        strlcpy((char *)c.sta.ssid, ssid, sizeof(c.sta.ssid));
        strlcpy((char *)c.sta.password, password ? password : "", sizeof(c.sta.password));
    } else {
        esp_wifi_get_config(WIFI_IF_STA, &c);
    }
    c.sta.listen_interval = LISTEN_INTERVAL;
    c.sta.pmf_cfg.capable = true;
    // Roaming between mesh access points: pick the strongest one.
    c.sta.scan_method = WIFI_ALL_CHANNEL_SCAN;
    c.sta.sort_method = WIFI_CONNECT_AP_BY_SIGNAL;
    esp_wifi_set_config(WIFI_IF_STA, &c);
}

esp_err_t dm_net_try_wifi(const char *ssid, const char *password, uint32_t timeout_ms)
{
    n.want_connection = true;
    n.retry_ms = 1000;
    esp_wifi_disconnect();
    configure_sta(ssid, password);
    xEventGroupClearBits(n.events, BIT_GOT_IP);
    wifi_connect();
    EventBits_t bits = xEventGroupWaitBits(n.events, BIT_GOT_IP, pdFALSE, pdTRUE, pdMS_TO_TICKS(timeout_ms));
    return (bits & BIT_GOT_IP) ? ESP_OK : ESP_ERR_TIMEOUT;
}

static void dispatch(const char *topic, const uint8_t *data, size_t len)
{
    if (strcmp(topic, n.topics.state) == 0) {
        if (n.cb.on_state) {
            n.cb.on_state((const char *)data, len);
        }
    } else if (strcmp(topic, n.topics.hub_online) == 0) {
        if (n.cb.on_hub_online) {
            n.cb.on_hub_online(len == 6 && memcmp(data, "online", 6) == 0);
        }
    } else if (strcmp(topic, n.topics.display) == 0) {
        if (n.cb.on_display) {
            n.cb.on_display(data, len);
        }
    } else if (strcmp(topic, n.topics.cmd) == 0) {
        if (n.cb.on_cmd) {
            n.cb.on_cmd((const char *)data, len);
        }
    }
}

static void on_data(const esp_mqtt_event_t *e)
{
    // Long messages arrive in chunks; only the first one carries the topic.
    if (e->current_data_offset == 0) {
        size_t tl = (size_t)e->topic_len < sizeof(n.rx_topic) - 1 ? (size_t)e->topic_len : sizeof(n.rx_topic) - 1;
        memcpy(n.rx_topic, e->topic, tl);
        n.rx_topic[tl] = '\0';
        n.rx_len = 0;
        n.rx_drop = e->total_data_len > RX_MAX;
        if (n.rx_drop) {
            ESP_LOGW(TAG, "%s: %d bytes is too large, dropped", n.rx_topic, e->total_data_len);
        }
    }
    if (n.rx_drop) {
        return;
    }
    memcpy(rx_buf + e->current_data_offset, e->data, e->data_len);
    n.rx_len = e->current_data_offset + e->data_len;
    if (n.rx_len == (size_t)e->total_data_len) {
        dispatch(n.rx_topic, rx_buf, n.rx_len);
    }
}

static void on_mqtt(void *arg, esp_event_base_t base, int32_t id, void *data)
{
    (void)arg;
    (void)base;
    esp_mqtt_event_handle_t e = data;
    switch ((esp_mqtt_event_id_t)id) {
    case MQTT_EVENT_CONNECTED:
        ESP_LOGI(TAG, "MQTT connected");
        esp_mqtt_client_publish(n.mqtt, n.topics.online, "online", 0, 1, 1);
        esp_mqtt_client_subscribe(n.mqtt, n.topics.state, 1);
        esp_mqtt_client_subscribe(n.mqtt, n.topics.hub_online, 1);
        esp_mqtt_client_subscribe(n.mqtt, n.topics.display, 1);
        esp_mqtt_client_subscribe(n.mqtt, n.topics.cmd, 1);
        n.mqtt_up = true;
        link_changed();
        break;
    case MQTT_EVENT_DISCONNECTED:
        ESP_LOGW(TAG, "MQTT disconnected");
        n.mqtt_up = false;
        link_changed();
        break;
    case MQTT_EVENT_DATA:
        on_data(e);
        break;
    case MQTT_EVENT_ERROR:
        // A failed connect only says why in esp-tls' error code (DNS,
        // timeout, refused …) – the socket errno stays 0 then.
        if (e->error_handle && e->error_handle->error_type == MQTT_ERROR_TYPE_CONNECTION_REFUSED) {
            snprintf(n.mqtt_error, sizeof(n.mqtt_error), "connack %d", e->error_handle->connect_return_code);
        } else if (e->error_handle && e->error_handle->esp_tls_last_esp_err != ESP_OK) {
            strlcpy(n.mqtt_error, esp_err_to_name(e->error_handle->esp_tls_last_esp_err), sizeof(n.mqtt_error));
            ESP_LOGW(TAG, "MQTT connection: %s", n.mqtt_error);
        }
        if (e->error_handle && e->error_handle->esp_transport_sock_errno != 0) {
            n.sock_errno = e->error_handle->esp_transport_sock_errno;
            ESP_LOGW(TAG, "MQTT socket error %d (%s)", n.sock_errno, strerror(n.sock_errno));
        }
        if (e->error_handle && e->error_handle->error_type == MQTT_ERROR_TYPE_CONNECTION_REFUSED) {
            ESP_LOGE(TAG, "MQTT refused the connection (code %d) – check mqtt_user/mqtt_pass",
                     e->error_handle->connect_return_code);
        }
        break;
    default:
        break;
    }
}

static void on_time_sync(struct timeval *tv)
{
    (void)tv;
    if (!n.clock_valid) {
        ESP_LOGI(TAG, "clock synchronised");
    }
    n.clock_valid = true;
}

esp_err_t dm_net_start(const dm_config_t *cfg, const dm_device_t *dev, const dm_net_callbacks_t *cb)
{
    n.cfg = cfg;
    n.dev = dev;
    n.cb = *cb;
    dm_topics(dev, &n.topics);

    // Modem sleep between beacons; with CONFIG_PM_ENABLE and tickless idle
    // the CPU drops into light sleep in between.
    configure_sta(NULL, NULL);
    ESP_RETURN_ON_ERROR(esp_wifi_set_ps(WIFI_PS_MAX_MODEM), TAG, "power save");
    n.want_connection = true;
    if (!n.wifi_up) {
        wifi_connect();
    }

    esp_sntp_config_t sntp = ESP_NETIF_SNTP_DEFAULT_CONFIG(cfg->ntp);
    sntp.sync_cb = on_time_sync;
    ESP_RETURN_ON_ERROR(esp_netif_sntp_init(&sntp), TAG, "sntp");

    esp_mqtt_client_config_t mc = {
        .broker.address.uri = cfg->mqtt_url,
        // mqtts:// and wss:// against a broker with a public certificate.
        .broker.verification.crt_bundle_attach = esp_crt_bundle_attach,
        .credentials.client_id = dev->id,
        .credentials.username = cfg->mqtt_user[0] ? cfg->mqtt_user : NULL,
        .credentials.authentication.password = cfg->mqtt_pass[0] ? cfg->mqtt_pass : NULL,
        .session.last_will = {.topic = n.topics.online, .msg = "offline", .qos = 1, .retain = 1},
        // A dead connection is noticed within ~1.5 keepalives; the failsafe
        // tolerates a longer outage anyway (stale_after).
        .session.keepalive = 60,
        .network.reconnect_timeout_ms = 5000,
    };
    n.mqtt = esp_mqtt_client_init(&mc);
    if (n.mqtt == NULL) {
        return ESP_FAIL;
    }
    ESP_RETURN_ON_ERROR(esp_mqtt_client_register_event(n.mqtt, ESP_EVENT_ANY_ID, on_mqtt, NULL), TAG, "mqtt events");
    atomic_store(&mqtt_created, true);
    if (xEventGroupGetBits(n.events) & BIT_GOT_IP) {
        start_mqtt();
    }
    return ESP_OK;
}

esp_err_t dm_net_publish(const char *topic, const char *payload, size_t len, bool retain)
{
    if (n.mqtt == NULL || !n.mqtt_up) {
        return ESP_ERR_INVALID_STATE;
    }
    return esp_mqtt_client_publish(n.mqtt, topic, payload, (int)len, 1, retain) < 0 ? ESP_FAIL : ESP_OK;
}

bool dm_net_connected(void)
{
    return n.wifi_up && n.mqtt_up;
}

void dm_net_kick(void)
{
    if (!n.want_connection) {
        return;
    }
    ESP_LOGW(TAG, "still offline (Wi-Fi %s, IP %s, MQTT %s) – restarting Wi-Fi", n.associated ? "up" : "down",
             n.wifi_up ? "yes" : "no", n.mqtt_up ? "up" : "down");
    n.retry_ms = 1000;
    if (n.associated) {
        // The DISCONNECTED event reconnects, and MQTT follows the new IP.
        n.kicked = true;
        esp_wifi_disconnect();
    } else {
        // Don't wait for the back-off: try now. If this doesn't help either,
        // the watchdog restarts the chip.
        esp_timer_stop(n.retry);
        wifi_connect();
    }
}

void dm_net_diag(dm_net_diag_t *d)
{
    d->associated = n.associated;
    d->ip = n.wifi_up;
    d->mqtt = n.mqtt_up;
    d->wifi_reason = n.wifi_reason;
    d->sock_errno = n.sock_errno;
    strlcpy(d->mqtt_error, n.mqtt_error, sizeof(d->mqtt_error));
}

int dm_net_rssi(void)
{
    wifi_ap_record_t ap;
    return n.wifi_up && esp_wifi_sta_get_ap_info(&ap) == ESP_OK ? ap.rssi : 0;
}

bool dm_net_clock_valid(void)
{
    return n.clock_valid;
}

int dm_net_scan(dm_net_ap_t *out, int max)
{
    wifi_scan_config_t sc = {.show_hidden = false};
    if (esp_wifi_scan_start(&sc, true) != ESP_OK) {
        return 0;
    }
    uint16_t count = (uint16_t)max;
    wifi_ap_record_t *recs = calloc(count, sizeof(*recs));
    if (recs == NULL) {
        esp_wifi_clear_ap_list();
        return 0;
    }
    esp_wifi_scan_get_ap_records(&count, recs);
    int k = 0;
    for (int i = 0; i < count; i++) {
        if (recs[i].ssid[0] == '\0') {
            continue;
        }
        strlcpy(out[k].ssid, (const char *)recs[i].ssid, sizeof(out[k].ssid));
        out[k].rssi = recs[i].rssi;
        out[k].secured = recs[i].authmode != WIFI_AUTH_OPEN;
        k++;
    }
    free(recs);
    return k;
}
