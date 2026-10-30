#include "check.h"
#include "dm_state.h"

// The example from the README (Home Assistant & MQTT), as the hub sends it.
static const char readme_state[] =
    "{\"status\":\"dnd\",\"phase\":\"ending_soon\",\"light\":\"red_yellow\",\"reason\":\"calendar\","
    "\"detail\":\"video_call\",\"since\":\"2026-10-28T14:00:00+01:00\",\"until\":\"2026-10-28T14:30:00+01:00\","
    "\"next\":{\"status\":\"working\",\"reason\":\"work_hours\",\"since\":\"2026-10-28T14:30:00+01:00\","
    "\"until\":\"2026-10-28T17:00:00+01:00\"},"
    "\"next_free\":{\"start\":\"2026-10-28T17:00:00+01:00\",\"end\":\"2026-10-29T08:00:00+01:00\"},"
    "\"override\":null,\"pomodoro\":null,\"updated\":\"2026-10-28T14:25:03.5+01:00\"}";

void test_state(void)
{
    dm_state_t s;
    CHECK_INT(dm_state_parse(readme_state, sizeof(readme_state) - 1, &s), DM_STATE_OK);
    CHECK_INT(s.status, DM_STATUS_DND);
    CHECK_INT(s.phase, DM_PHASE_ENDING_SOON);
    CHECK_INT(s.light, DM_LIGHT_RED_YELLOW);
    CHECK(s.light_known);
    CHECK_STR(s.reason, "calendar");
    CHECK_STR(s.detail, "video_call");
    CHECK_INT(s.since, 1793192400);
    CHECK_INT(s.until, 1793192400 + 1800);
    CHECK_INT(s.next_free_start, 1793192400 + 3 * 3600);
    CHECK_INT(s.next_free_end, 1793192400 + 18 * 3600);
    CHECK_INT(s.updated, 1793192400 + 25 * 60 + 3);
    CHECK_STR(s.message, "");

    // Every light value round-trips through its name.
    for (int i = 0; i < DM_LIGHT_COUNT; i++) {
        bool known = false;
        CHECK_INT(dm_light_parse(dm_light_name((dm_light_t)i), &known), i);
        CHECK(known);
    }

    // Until further notice, a message, no next_free.
    const char indefinite[] = "{\"status\":\"working\",\"phase\":\"normal\",\"light\":\"yellow\",\"reason\":\"override\","
                              "\"until\":null,\"next_free\":null,\"message\":\"Back at 3 \\u2615\"}";
    CHECK_INT(dm_state_parse(indefinite, sizeof(indefinite) - 1, &s), DM_STATE_OK);
    CHECK_INT(s.until, 0);
    CHECK_INT(s.next_free_start, 0);
    CHECK_STR(s.message, "Back at 3 \xe2\x98\x95");
    CHECK_INT(s.light, DM_LIGHT_YELLOW);

    // A newer hub with a pattern this firmware doesn't know: parsed, but flagged.
    const char future[] = "{\"status\":\"dnd\",\"light\":\"purple_pulse\"}";
    CHECK_INT(dm_state_parse(future, sizeof(future) - 1, &s), DM_STATE_OK);
    CHECK(!s.light_known);
    CHECK_INT(s.phase, DM_PHASE_NORMAL);

    // Broken documents.
    CHECK_INT(dm_state_parse("not json", 8, &s), DM_STATE_ERR_JSON);
    CHECK_INT(dm_state_parse("[1,2]", 5, &s), DM_STATE_ERR_JSON);
    CHECK_INT(dm_state_parse("{\"status\":\"dnd\"}", 16, &s), DM_STATE_ERR_FIELDS);
    // Length is honoured (MQTT payloads are not NUL-terminated).
    CHECK_INT(dm_state_parse(readme_state, 20, &s), DM_STATE_ERR_JSON);

    // A long message is cut on a UTF-8 boundary: 100 × "€" (3 bytes each).
    char big[1024];
    int n = snprintf(big, sizeof(big), "{\"status\":\"free\",\"light\":\"green\",\"message\":\"");
    for (int i = 0; i < 100; i++) {
        n += snprintf(big + n, sizeof(big) - (size_t)n, "\xe2\x82\xac");
    }
    n += snprintf(big + n, sizeof(big) - (size_t)n, "\"}");
    CHECK_INT(dm_state_parse(big, (size_t)n, &s), DM_STATE_OK);
    CHECK_INT(strlen(s.message), 255); // 85 whole characters, not 256 bytes with a broken one
}

void test_failsafe(void)
{
    dm_failsafe_cfg_t cfg = DM_FAILSAFE_DEFAULTS;
    dm_state_t s;
    const char doc[] = "{\"status\":\"dnd\",\"light\":\"red\",\"until\":\"2026-10-28T14:30:00+01:00\"}";
    CHECK_INT(dm_state_parse(doc, sizeof(doc) - 1, &s), DM_STATE_OK);
    const int64_t until = 1793192400 + 1800;

    dm_link_t ok = {
        .now_ms = 300000, .boot_ms = 0, .have_state = true, .hub_online_known = true, .hub_online = true,
        .connected = true, .clock_valid = true, .now_unix = until - 60,
    };
    CHECK_INT(dm_failsafe_check(&cfg, &ok, &s), DM_FAULT_NONE);
    CHECK_INT(dm_effective_light(DM_FAULT_NONE, &s), DM_LIGHT_RED);

    // Just booted, nothing received: dark, then fault after the grace period.
    dm_link_t l = ok;
    l.have_state = false;
    l.hub_online_known = false;
    l.now_ms = 10000;
    CHECK_INT(dm_failsafe_check(&cfg, &l, NULL), DM_FAULT_BOOTING);
    CHECK_INT(dm_effective_light(DM_FAULT_BOOTING, &s), DM_LIGHT_OFF);
    l.now_ms = 60000;
    CHECK_INT(dm_failsafe_check(&cfg, &l, NULL), DM_FAULT_NO_STATE);
    CHECK_INT(dm_effective_light(DM_FAULT_NO_STATE, &s), DM_LIGHT_YELLOW_BLINK);

    // The hub's Last Will beats a perfectly fresh state.
    l = ok;
    l.hub_online = false;
    CHECK_INT(dm_failsafe_check(&cfg, &l, &s), DM_FAULT_HUB_OFFLINE);

    // Short connection loss is tolerated, a long one isn't.
    l = ok;
    l.connected = false;
    l.disconnected_ms = l.now_ms - 119000;
    CHECK_INT(dm_failsafe_check(&cfg, &l, &s), DM_FAULT_NONE);
    l.disconnected_ms = l.now_ms - 120000;
    CHECK_INT(dm_failsafe_check(&cfg, &l, &s), DM_FAULT_DISCONNECTED);

    // `until` long past: the hub should have sent the next state.
    l = ok;
    l.now_unix = until + 120;
    CHECK_INT(dm_failsafe_check(&cfg, &l, &s), DM_FAULT_NONE);
    l.now_unix = until + 121;
    CHECK_INT(dm_failsafe_check(&cfg, &l, &s), DM_FAULT_EXPIRED);
    // … but only with a valid clock.
    l.clock_valid = false;
    CHECK_INT(dm_failsafe_check(&cfg, &l, &s), DM_FAULT_NONE);

    // Until further notice never expires.
    dm_state_t forever = s;
    forever.until = 0;
    l = ok;
    l.now_unix = until + 86400;
    CHECK_INT(dm_failsafe_check(&cfg, &l, &forever), DM_FAULT_NONE);

    // Unknown pattern → fault.
    dm_state_t weird = s;
    weird.light_known = false;
    CHECK_INT(dm_failsafe_check(&cfg, &ok, &weird), DM_FAULT_UNKNOWN_LIGHT);

    for (int f = DM_FAULT_NONE; f <= DM_FAULT_UNKNOWN_LIGHT; f++) {
        CHECK(strcmp(dm_fault_name((dm_fault_t)f), "unknown") != 0);
    }
}
