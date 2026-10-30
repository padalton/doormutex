#include "dm_discovery.h"

#include <stdio.h>
#include <string.h>

#include "cJSON.h"

#define SUPPORT_URL "https://github.com/padalton/doormutex"

void dm_device_id(const uint8_t mac[6], char *out, size_t cap)
{
    snprintf(out, cap, "doormutex-%02x%02x%02x", mac[3], mac[4], mac[5]);
}

void dm_topics(const dm_device_t *dev, dm_topics_t *t)
{
    snprintf(t->telemetry, sizeof(t->telemetry), "%s/device/%s/telemetry", dev->base_topic, dev->id);
    snprintf(t->online, sizeof(t->online), "%s/device/%s/online", dev->base_topic, dev->id);
    snprintf(t->cmd, sizeof(t->cmd), "%s/device/%s/cmd", dev->base_topic, dev->id);
    snprintf(t->state, sizeof(t->state), "%s/%s/state", dev->base_topic, dev->profile);
    snprintf(t->hub_online, sizeof(t->hub_online), "%s/hub/online", dev->base_topic);
    snprintf(t->display, sizeof(t->display), "%s/%s/display", dev->base_topic, dev->profile);
}

static size_t print_into(cJSON *root, char *out, size_t cap)
{
    // Print straight into the caller's buffer – no heap copy of the text.
    bool ok = cJSON_PrintPreallocated(root, out, (int)cap, false);
    cJSON_Delete(root);
    return ok ? strlen(out) : 0;
}

size_t dm_telemetry_json(const dm_telemetry_t *t, char *out, size_t cap)
{
    cJSON *root = cJSON_CreateObject();
    if (t->battery_pct >= 0) {
        // one decimal is plenty for a fuel gauge
        cJSON_AddNumberToObject(root, "battery", (double)((int)(t->battery_pct * 10.0f + 0.5f)) / 10.0);
    } else {
        cJSON_AddNullToObject(root, "battery");
    }
    if (t->battery_mv > 0) {
        cJSON_AddNumberToObject(root, "voltage", t->battery_mv / 1000.0);
    } else {
        cJSON_AddNullToObject(root, "voltage");
    }
    cJSON_AddBoolToObject(root, "charging", t->charging);
    cJSON_AddBoolToObject(root, "usb", t->usb);
    if (t->rssi != 0) {
        cJSON_AddNumberToObject(root, "rssi", t->rssi);
    } else {
        cJSON_AddNullToObject(root, "rssi");
    }
    cJSON_AddStringToObject(root, "light", t->light ? t->light : "off");
    cJSON_AddStringToObject(root, "fault", t->fault ? t->fault : "none");
    cJSON_AddNumberToObject(root, "uptime", t->uptime_s);
    cJSON_AddStringToObject(root, "firmware", t->firmware ? t->firmware : "");
    return print_into(root, out, cap);
}

// entity adds one component; ids follow the hub's naming: unique_id from
// the device (stable per light), entity ID from the profile.
static cJSON *entity(cJSON *components, const dm_device_t *dev, const char *base, const char *platform,
                     const char *key, const char *name)
{
    char buf[128];
    cJSON *e = cJSON_AddObjectToObject(components, key);
    cJSON_AddStringToObject(e, "platform", platform);
    snprintf(buf, sizeof(buf), "%s_%s", base, key);
    cJSON_AddStringToObject(e, "unique_id", buf);
    snprintf(buf, sizeof(buf), "%s.doormutex_%s_%s", platform, dev->profile, key);
    for (char *p = buf; *p; p++) {
        if (*p == '-') {
            *p = '_';
        }
    }
    cJSON_AddStringToObject(e, "default_entity_id", buf);
    cJSON_AddStringToObject(e, "name", name);
    return e;
}

size_t dm_discovery(const dm_device_t *dev, char *topic, size_t topic_cap, char *out, size_t cap)
{
    dm_topics_t t;
    dm_topics(dev, &t);
    char base[48];
    snprintf(base, sizeof(base), "%s", dev->id);
    for (char *p = base; *p; p++) {
        if (*p == '-') {
            *p = '_';
        }
    }
    snprintf(topic, topic_cap, "%s/device/%s/config", dev->discovery_prefix, base);

    cJSON *root = cJSON_CreateObject();
    cJSON *device = cJSON_AddObjectToObject(root, "device");
    cJSON *ids = cJSON_AddArrayToObject(device, "identifiers");
    cJSON_AddItemToArray(ids, cJSON_CreateString(base));
    char name[64];
    snprintf(name, sizeof(name), "doormutex light %s", dev->profile);
    cJSON_AddStringToObject(device, "name", name);
    cJSON_AddStringToObject(device, "manufacturer", "doormutex");
    cJSON_AddStringToObject(device, "model", dev->model);
    cJSON_AddStringToObject(device, "sw_version", dev->firmware);
    // Link the light to its profile's hub device in HA's device page.
    char via[48];
    snprintf(via, sizeof(via), "doormutex_%s", dev->profile);
    for (char *p = via; *p; p++) {
        if (*p == '-') {
            *p = '_';
        }
    }
    cJSON_AddStringToObject(device, "via_device", via);

    cJSON *origin = cJSON_AddObjectToObject(root, "origin");
    cJSON_AddStringToObject(origin, "name", "doormutex firmware");
    cJSON_AddStringToObject(origin, "sw_version", dev->firmware);
    cJSON_AddStringToObject(origin, "support_url", SUPPORT_URL);

    cJSON_AddStringToObject(root, "availability_topic", t.online);
    cJSON_AddStringToObject(root, "state_topic", t.telemetry);
    cJSON_AddNumberToObject(root, "qos", 1);

    cJSON *c = cJSON_AddObjectToObject(root, "components");
    cJSON *e = entity(c, dev, base, "sensor", "battery", "Battery");
    cJSON_AddStringToObject(e, "device_class", "battery");
    cJSON_AddStringToObject(e, "state_class", "measurement");
    cJSON_AddStringToObject(e, "unit_of_measurement", "%");
    cJSON_AddStringToObject(e, "value_template", "{{ value_json.battery }}");

    e = entity(c, dev, base, "sensor", "battery_voltage", "Battery voltage");
    cJSON_AddStringToObject(e, "device_class", "voltage");
    cJSON_AddStringToObject(e, "state_class", "measurement");
    cJSON_AddStringToObject(e, "unit_of_measurement", "V");
    cJSON_AddStringToObject(e, "entity_category", "diagnostic");
    cJSON_AddStringToObject(e, "value_template", "{{ value_json.voltage }}");

    e = entity(c, dev, base, "binary_sensor", "charging", "Charging");
    cJSON_AddStringToObject(e, "device_class", "battery_charging");
    cJSON_AddStringToObject(e, "value_template", "{{ 'ON' if value_json.charging else 'OFF' }}");

    e = entity(c, dev, base, "binary_sensor", "usb_power", "USB power");
    cJSON_AddStringToObject(e, "device_class", "plug");
    cJSON_AddStringToObject(e, "entity_category", "diagnostic");
    cJSON_AddStringToObject(e, "value_template", "{{ 'ON' if value_json.usb else 'OFF' }}");

    e = entity(c, dev, base, "sensor", "rssi", "Wi-Fi signal");
    cJSON_AddStringToObject(e, "device_class", "signal_strength");
    cJSON_AddStringToObject(e, "state_class", "measurement");
    cJSON_AddStringToObject(e, "unit_of_measurement", "dBm");
    cJSON_AddStringToObject(e, "entity_category", "diagnostic");
    cJSON_AddStringToObject(e, "value_template", "{{ value_json.rssi }}");

    e = entity(c, dev, base, "sensor", "light_fault", "Light fault");
    cJSON_AddStringToObject(e, "icon", "mdi:traffic-light-outline");
    cJSON_AddStringToObject(e, "entity_category", "diagnostic");
    cJSON_AddStringToObject(e, "value_template", "{{ value_json.fault }}");

    e = entity(c, dev, base, "button", "identify", "Identify");
    cJSON_AddStringToObject(e, "device_class", "identify");
    cJSON_AddStringToObject(e, "entity_category", "config");
    cJSON_AddStringToObject(e, "command_topic", t.cmd);
    cJSON_AddStringToObject(e, "payload_press", "identify");

    e = entity(c, dev, base, "button", "restart", "Restart");
    cJSON_AddStringToObject(e, "device_class", "restart");
    cJSON_AddStringToObject(e, "entity_category", "config");
    cJSON_AddStringToObject(e, "command_topic", t.cmd);
    cJSON_AddStringToObject(e, "payload_press", "restart");

    return print_into(root, out, cap);
}
