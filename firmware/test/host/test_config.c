#include "check.h"
#include "dm_config.h"
#include "dm_pattern.h"

void test_config(void)
{
    dm_config_t c;
    dm_config_defaults(&c);
    CHECK_STR(c.profile, "office");
    CHECK_STR(c.base_topic, "doormutex");
    CHECK(!dm_config_ready(&c));

    CHECK_INT(dm_config_set(&c, "mqtt_url", "mqtt://broker.local:1883"), DM_CONFIG_OK);
    CHECK(dm_config_ready(&c));
    CHECK_INT(dm_config_set(&c, "mqtt_url", "http://broker"), DM_CONFIG_INVALID);
    CHECK_STR(c.mqtt_url, "mqtt://broker.local:1883");
    CHECK_INT(dm_config_set(&c, "profile", "teen"), DM_CONFIG_OK);
    CHECK_INT(dm_config_set(&c, "profile", "Teen Room"), DM_CONFIG_INVALID);
    CHECK_INT(dm_config_set(&c, "profile", "a/b"), DM_CONFIG_INVALID);
    CHECK_INT(dm_config_set(&c, "brightness", "100"), DM_CONFIG_OK);
    CHECK_INT(c.brightness, 100);
    CHECK_INT(dm_config_set(&c, "brightness", "0"), DM_CONFIG_INVALID);
    CHECK_INT(dm_config_set(&c, "brightness", "50%"), DM_CONFIG_INVALID);
    CHECK_INT(dm_config_set(&c, "colour", "red"), DM_CONFIG_UNKNOWN_KEY);

    // Empty resets to the default.
    CHECK_INT(dm_config_set(&c, "profile", ""), DM_CONFIG_OK);
    CHECK_STR(c.profile, "office");

    // Secrets are masked in `show`.
    char out[128];
    CHECK_INT(dm_config_set(&c, "mqtt_pass", "hunter2 with space"), DM_CONFIG_OK);
    CHECK_INT(dm_config_get(&c, "mqtt_pass", true, out, sizeof(out)), DM_CONFIG_OK);
    CHECK_STR(out, "***");
    CHECK_INT(dm_config_get(&c, "mqtt_pass", false, out, sizeof(out)), DM_CONFIG_OK);
    CHECK_STR(out, "hunter2 with space");
    for (size_t i = 0; i < dm_config_key_count; i++) {
        CHECK_INT(dm_config_get(&c, dm_config_keys[i], true, out, sizeof(out)), DM_CONFIG_OK);
        // NVS rejects longer keys (ESP_ERR_NVS_KEY_TOO_LONG): nothing would be saved.
        CHECK(strlen(dm_config_nvs_keys[i]) <= 15);
    }
    char longv[200];
    memset(longv, 'x', sizeof(longv) - 1);
    longv[sizeof(longv) - 1] = '\0';
    CHECK_INT(dm_config_set(&c, "mqtt_user", longv), DM_CONFIG_INVALID);

    // Console lines.
    dm_cmd_t cmd;
    dm_console_parse("  set mqtt_pass hunter2 with space\r\n", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_SET);
    CHECK_STR(cmd.key, "mqtt_pass");
    CHECK_STR(cmd.value, "hunter2 with space");
    dm_console_parse("set profile", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_SET);
    CHECK_STR(cmd.value, "");
    dm_console_parse("set", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_UNKNOWN);
    dm_console_parse("show\n", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_SHOW);
    dm_console_parse("\r\n", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_NONE);
    dm_console_parse("lamps ryg\n", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_LAMPS);
    CHECK_STR(cmd.key, "ryg");
    dm_console_parse("lamps", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_LAMPS);
    CHECK_STR(cmd.key, "");
    CHECK_INT(dm_lamps_parse("ryg"), DM_LAMP_RED | DM_LAMP_YELLOW | DM_LAMP_GREEN);
    CHECK_INT(dm_lamps_parse("y"), DM_LAMP_YELLOW);
    CHECK_INT(dm_lamps_parse("gr"), DM_LAMP_RED | DM_LAMP_GREEN);
    CHECK_INT(dm_lamps_parse("all"), DM_LAMP_RED | DM_LAMP_YELLOW | DM_LAMP_GREEN);
    CHECK_INT(dm_lamps_parse("off"), 0);
    CHECK_INT(dm_lamps_parse("auto"), DM_LAMPS_AUTO);
    CHECK_INT(dm_lamps_parse(""), DM_LAMPS_AUTO);
    CHECK_INT(dm_lamps_parse("rx"), DM_LAMPS_INVALID);
    dm_console_parse("factory-reset", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_FACTORY_RESET);
    dm_console_parse("identify", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_IDENTIFY);
    dm_console_parse("reboot", &cmd);
    CHECK_INT(cmd.kind, DM_CMD_UNKNOWN);
}
