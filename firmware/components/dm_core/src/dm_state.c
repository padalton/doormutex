#include "dm_state.h"

#include <string.h>

#include "cJSON.h"
#include "dm_time.h"

static const char *const light_names[DM_LIGHT_COUNT] = {
    [DM_LIGHT_OFF] = "off",
    [DM_LIGHT_RED] = "red",
    [DM_LIGHT_YELLOW] = "yellow",
    [DM_LIGHT_GREEN] = "green",
    [DM_LIGHT_GREEN_BLINK] = "green_blink",
    [DM_LIGHT_RED_YELLOW] = "red_yellow",
    [DM_LIGHT_RED_YELLOW_ALTERNATE] = "red_yellow_alternate",
    [DM_LIGHT_YELLOW_BLINK] = "yellow_blink",
};

dm_light_t dm_light_parse(const char *s, bool *known)
{
    for (int i = 0; s != NULL && i < DM_LIGHT_COUNT; i++) {
        if (strcmp(s, light_names[i]) == 0) {
            *known = true;
            return (dm_light_t)i;
        }
    }
    *known = false;
    return DM_LIGHT_YELLOW_BLINK;
}

const char *dm_light_name(dm_light_t light)
{
    return light < DM_LIGHT_COUNT ? light_names[light] : "unknown";
}

dm_status_t dm_status_parse(const char *s)
{
    if (s == NULL) {
        return DM_STATUS_UNKNOWN;
    }
    if (strcmp(s, "free") == 0) {
        return DM_STATUS_FREE;
    }
    if (strcmp(s, "working") == 0) {
        return DM_STATUS_WORKING;
    }
    if (strcmp(s, "dnd") == 0) {
        return DM_STATUS_DND;
    }
    if (strcmp(s, "away") == 0) {
        return DM_STATUS_AWAY;
    }
    return DM_STATUS_UNKNOWN;
}

dm_phase_t dm_phase_parse(const char *s)
{
    static const struct {
        const char *name;
        dm_phase_t phase;
    } phases[] = {
        {"normal", DM_PHASE_NORMAL},       {"last_call", DM_PHASE_LAST_CALL}, {"amber", DM_PHASE_AMBER},
        {"ending_soon", DM_PHASE_ENDING_SOON}, {"fault", DM_PHASE_FAULT},
    };
    for (size_t i = 0; s != NULL && i < sizeof(phases) / sizeof(phases[0]); i++) {
        if (strcmp(s, phases[i].name) == 0) {
            return phases[i].phase;
        }
    }
    return DM_PHASE_UNKNOWN;
}

static void copy_string(const cJSON *obj, const char *key, char *dst, size_t cap)
{
    const cJSON *v = cJSON_GetObjectItemCaseSensitive(obj, key);
    dst[0] = '\0';
    if (cJSON_IsString(v) && v->valuestring != NULL) {
        strncpy(dst, v->valuestring, cap - 1);
        dst[cap - 1] = '\0';
        // Don't leave half a UTF-8 sequence at the end of a cut string.
        size_t n = strlen(dst);
        if (n == cap - 1) {
            size_t i = n;
            while (i > 0 && ((unsigned char)dst[i - 1] & 0xC0) == 0x80) {
                i--;
            }
            if (i > 0 && ((unsigned char)dst[i - 1] & 0x80)) {
                unsigned char lead = (unsigned char)dst[i - 1];
                size_t want = lead >= 0xF0 ? 4 : lead >= 0xE0 ? 3 : 2;
                if (n - (i - 1) < want) {
                    dst[i - 1] = '\0';
                }
            }
        }
    }
}

// time_field reads an RFC 3339 string; null, missing or malformed → 0.
static int64_t time_field(const cJSON *obj, const char *key)
{
    const cJSON *v = cJSON_GetObjectItemCaseSensitive(obj, key);
    int64_t t;
    if (cJSON_IsString(v) && dm_parse_rfc3339(v->valuestring, &t) == 0) {
        return t;
    }
    return 0;
}

int dm_state_parse(const char *json, size_t len, dm_state_t *out)
{
    memset(out, 0, sizeof(*out));
    cJSON *root = cJSON_ParseWithLength(json, len);
    if (!cJSON_IsObject(root)) {
        cJSON_Delete(root);
        return DM_STATE_ERR_JSON;
    }
    const cJSON *status = cJSON_GetObjectItemCaseSensitive(root, "status");
    const cJSON *light = cJSON_GetObjectItemCaseSensitive(root, "light");
    if (!cJSON_IsString(status) || !cJSON_IsString(light)) {
        cJSON_Delete(root);
        return DM_STATE_ERR_FIELDS;
    }
    out->status = dm_status_parse(status->valuestring);
    const cJSON *phase = cJSON_GetObjectItemCaseSensitive(root, "phase");
    out->phase = cJSON_IsString(phase) ? dm_phase_parse(phase->valuestring) : DM_PHASE_NORMAL;
    out->light = dm_light_parse(light->valuestring, &out->light_known);
    copy_string(root, "reason", out->reason, sizeof(out->reason));
    copy_string(root, "detail", out->detail, sizeof(out->detail));
    copy_string(root, "message", out->message, sizeof(out->message));
    out->since = time_field(root, "since");
    out->until = time_field(root, "until");
    out->updated = time_field(root, "updated");
    const cJSON *nf = cJSON_GetObjectItemCaseSensitive(root, "next_free");
    if (cJSON_IsObject(nf)) {
        out->next_free_start = time_field(nf, "start");
        out->next_free_end = time_field(nf, "end");
    }
    cJSON_Delete(root);
    return DM_STATE_OK;
}

dm_fault_t dm_failsafe_check(const dm_failsafe_cfg_t *cfg, const dm_link_t *link, const dm_state_t *state)
{
    // The hub's Last Will beats everything: its state is frozen.
    if (link->hub_online_known && !link->hub_online) {
        return DM_FAULT_HUB_OFFLINE;
    }
    if (!link->connected && link->now_ms - link->disconnected_ms >= cfg->stale_after_ms) {
        return DM_FAULT_DISCONNECTED;
    }
    if (!link->have_state || state == NULL) {
        return link->now_ms - link->boot_ms < cfg->boot_grace_ms ? DM_FAULT_BOOTING : DM_FAULT_NO_STATE;
    }
    if (!state->light_known) {
        return DM_FAULT_UNKNOWN_LIGHT;
    }
    // The hub publishes only on change, so `updated` may legitimately be
    // hours old. But it always publishes when `until` is reached – if that
    // is long past, the hub is stuck even though MQTT still looks fine.
    if (link->clock_valid && state->until != 0 && link->now_unix > state->until + (int64_t)cfg->until_grace_s) {
        return DM_FAULT_EXPIRED;
    }
    return DM_FAULT_NONE;
}

dm_light_t dm_effective_light(dm_fault_t fault, const dm_state_t *state)
{
    switch (fault) {
    case DM_FAULT_NONE:
        return state->light;
    case DM_FAULT_BOOTING:
        return DM_LIGHT_OFF;
    default:
        return DM_LIGHT_YELLOW_BLINK;
    }
}

const char *dm_fault_name(dm_fault_t fault)
{
    switch (fault) {
    case DM_FAULT_NONE:
        return "none";
    case DM_FAULT_BOOTING:
        return "booting";
    case DM_FAULT_NO_STATE:
        return "no_state";
    case DM_FAULT_HUB_OFFLINE:
        return "hub_offline";
    case DM_FAULT_DISCONNECTED:
        return "disconnected";
    case DM_FAULT_EXPIRED:
        return "expired";
    case DM_FAULT_UNKNOWN_LIGHT:
        return "unknown_light";
    }
    return "unknown";
}
