#include "cJSON.h"
#include "check.h"
#include "dm_discovery.h"

static const char *str(const cJSON *o, const char *k)
{
    const cJSON *v = cJSON_GetObjectItemCaseSensitive(o, k);
    return cJSON_IsString(v) ? v->valuestring : NULL;
}

void test_discovery(void)
{
    const uint8_t mac[6] = {0x40, 0x4c, 0xca, 0xa1, 0xb2, 0xc3};
    dm_device_t dev = {
        .profile = "teen-room", .base_topic = "doormutex", .discovery_prefix = "homeassistant",
        .firmware = "0.1.0", .model = "ESP32-C6 traffic light",
    };
    dm_device_id(mac, dev.id, sizeof(dev.id));
    CHECK_STR(dev.id, "doormutex-a1b2c3");

    dm_topics_t t;
    dm_topics(&dev, &t);
    CHECK_STR(t.state, "doormutex/teen-room/state");
    CHECK_STR(t.hub_online, "doormutex/hub/online");
    CHECK_STR(t.display, "doormutex/teen-room/display");
    CHECK_STR(t.telemetry, "doormutex/device/doormutex-a1b2c3/telemetry");
    CHECK_STR(t.online, "doormutex/device/doormutex-a1b2c3/online");
    CHECK_STR(t.cmd, "doormutex/device/doormutex-a1b2c3/cmd");

    char topic[128], payload[4096];
    size_t n = dm_discovery(&dev, topic, sizeof(topic), payload, sizeof(payload));
    CHECK(n > 0);
    CHECK_STR(topic, "homeassistant/device/doormutex_a1b2c3/config");
    cJSON *root = cJSON_Parse(payload);
    CHECK(root != NULL);
    const cJSON *device = cJSON_GetObjectItemCaseSensitive(root, "device");
    CHECK_STR(str(device, "via_device"), "doormutex_teen_room"); // the hub's profile device
    CHECK_STR(str(device, "sw_version"), "0.1.0");
    CHECK_STR(str(root, "availability_topic"), t.online);
    CHECK_STR(str(root, "state_topic"), t.telemetry);
    const cJSON *c = cJSON_GetObjectItemCaseSensitive(root, "components");
    const cJSON *battery = cJSON_GetObjectItemCaseSensitive(c, "battery");
    CHECK_STR(str(battery, "default_entity_id"), "sensor.doormutex_teen_room_battery");
    CHECK_STR(str(battery, "unique_id"), "doormutex_a1b2c3_battery");
    CHECK_STR(str(battery, "device_class"), "battery");
    const cJSON *charging = cJSON_GetObjectItemCaseSensitive(c, "charging");
    CHECK_STR(str(charging, "default_entity_id"), "binary_sensor.doormutex_teen_room_charging");
    const cJSON *identify = cJSON_GetObjectItemCaseSensitive(c, "identify");
    CHECK_STR(str(identify, "command_topic"), t.cmd);
    CHECK_STR(str(identify, "payload_press"), "identify");
    cJSON_Delete(root);

    // Too small a buffer fails cleanly.
    CHECK_INT(dm_discovery(&dev, topic, sizeof(topic), payload, 64), 0);

    // Telemetry: unknown values become null.
    dm_telemetry_t tel = {.battery_pct = 87.46f, .battery_mv = 3912, .charging = true, .usb = true, .rssi = -61,
                          .light = "red", .fault = "none", .uptime_s = 42, .firmware = "0.1.0"};
    char doc[512];
    CHECK(dm_telemetry_json(&tel, doc, sizeof(doc)) > 0);
    root = cJSON_Parse(doc);
    CHECK(cJSON_GetObjectItemCaseSensitive(root, "battery")->valuedouble > 87.49 &&
          cJSON_GetObjectItemCaseSensitive(root, "battery")->valuedouble < 87.51);
    CHECK(cJSON_GetObjectItemCaseSensitive(root, "voltage")->valuedouble > 3.911);
    CHECK(cJSON_IsTrue(cJSON_GetObjectItemCaseSensitive(root, "charging")));
    CHECK_INT(cJSON_GetObjectItemCaseSensitive(root, "rssi")->valueint, -61);
    CHECK_STR(str(root, "light"), "red");
    cJSON_Delete(root);

    dm_telemetry_t none = {.battery_pct = -1, .light = "off", .fault = "booting", .firmware = "0.1.0"};
    CHECK(dm_telemetry_json(&none, doc, sizeof(doc)) > 0);
    root = cJSON_Parse(doc);
    CHECK(cJSON_IsNull(cJSON_GetObjectItemCaseSensitive(root, "battery")));
    CHECK(cJSON_IsNull(cJSON_GetObjectItemCaseSensitive(root, "voltage")));
    CHECK(cJSON_IsNull(cJSON_GetObjectItemCaseSensitive(root, "rssi")));
    cJSON_Delete(root);
}
