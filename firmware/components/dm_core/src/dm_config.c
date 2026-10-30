#include "dm_config.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

const char *const dm_config_keys[] = {
    "mqtt_url", "mqtt_user", "mqtt_pass", "profile", "base_topic", "discovery_prefix", "ntp", "brightness",
};
const size_t dm_config_key_count = sizeof(dm_config_keys) / sizeof(dm_config_keys[0]);

void dm_config_defaults(dm_config_t *c)
{
    memset(c, 0, sizeof(*c));
    strcpy(c->profile, "office");
    strcpy(c->base_topic, "doormutex");
    strcpy(c->discovery_prefix, "homeassistant");
    strcpy(c->ntp, "pool.ntp.org");
    c->brightness = 40;
}

static bool has_prefix(const char *s, const char *p)
{
    return strncmp(s, p, strlen(p)) == 0;
}

// topic_segment: profile and topic names are lower-case words, like the hub's.
static bool topic_segment(const char *s)
{
    if (*s == '\0') {
        return false;
    }
    for (; *s; s++) {
        if (!((*s >= 'a' && *s <= 'z') || (*s >= '0' && *s <= '9') || *s == '_' || *s == '-')) {
            return false;
        }
    }
    return true;
}

static int set_string(char *dst, size_t cap, const char *value)
{
    if (strlen(value) >= cap) {
        return DM_CONFIG_INVALID;
    }
    strcpy(dst, value);
    return DM_CONFIG_OK;
}

int dm_config_set(dm_config_t *c, const char *key, const char *value)
{
    dm_config_t def;
    dm_config_defaults(&def);
    bool reset = value == NULL || *value == '\0';

    if (strcmp(key, "mqtt_url") == 0) {
        if (!reset && !has_prefix(value, "mqtt://") && !has_prefix(value, "mqtts://") && !has_prefix(value, "ws://") &&
            !has_prefix(value, "wss://")) {
            return DM_CONFIG_INVALID;
        }
        return set_string(c->mqtt_url, sizeof(c->mqtt_url), reset ? def.mqtt_url : value);
    }
    if (strcmp(key, "mqtt_user") == 0) {
        return set_string(c->mqtt_user, sizeof(c->mqtt_user), reset ? def.mqtt_user : value);
    }
    if (strcmp(key, "mqtt_pass") == 0) {
        return set_string(c->mqtt_pass, sizeof(c->mqtt_pass), reset ? def.mqtt_pass : value);
    }
    if (strcmp(key, "profile") == 0 || strcmp(key, "base_topic") == 0 || strcmp(key, "discovery_prefix") == 0) {
        char *dst = key[0] == 'p' ? c->profile : key[0] == 'b' ? c->base_topic : c->discovery_prefix;
        const char *d = key[0] == 'p' ? def.profile : key[0] == 'b' ? def.base_topic : def.discovery_prefix;
        if (!reset && !topic_segment(value)) {
            return DM_CONFIG_INVALID;
        }
        return set_string(dst, sizeof(c->profile), reset ? d : value);
    }
    if (strcmp(key, "ntp") == 0) {
        return set_string(c->ntp, sizeof(c->ntp), reset ? def.ntp : value);
    }
    if (strcmp(key, "brightness") == 0) {
        if (reset) {
            c->brightness = def.brightness;
            return DM_CONFIG_OK;
        }
        char *end;
        long v = strtol(value, &end, 10);
        if (*end != '\0' || v < 1 || v > 100) {
            return DM_CONFIG_INVALID;
        }
        c->brightness = (uint8_t)v;
        return DM_CONFIG_OK;
    }
    return DM_CONFIG_UNKNOWN_KEY;
}

bool dm_config_ready(const dm_config_t *c)
{
    return c->mqtt_url[0] != '\0' && c->profile[0] != '\0';
}

int dm_config_get(const dm_config_t *c, const char *key, bool mask, char *out, size_t cap)
{
    const char *v = NULL;
    if (strcmp(key, "mqtt_url") == 0) {
        v = c->mqtt_url;
    } else if (strcmp(key, "mqtt_user") == 0) {
        v = c->mqtt_user;
    } else if (strcmp(key, "mqtt_pass") == 0) {
        v = mask && c->mqtt_pass[0] ? "***" : c->mqtt_pass;
    } else if (strcmp(key, "profile") == 0) {
        v = c->profile;
    } else if (strcmp(key, "base_topic") == 0) {
        v = c->base_topic;
    } else if (strcmp(key, "discovery_prefix") == 0) {
        v = c->discovery_prefix;
    } else if (strcmp(key, "ntp") == 0) {
        v = c->ntp;
    } else if (strcmp(key, "brightness") == 0) {
        snprintf(out, cap, "%u", c->brightness);
        return DM_CONFIG_OK;
    } else {
        return DM_CONFIG_UNKNOWN_KEY;
    }
    snprintf(out, cap, "%s", v);
    return DM_CONFIG_OK;
}

void dm_console_parse(const char *line, dm_cmd_t *cmd)
{
    memset(cmd, 0, sizeof(*cmd));
    while (*line == ' ' || *line == '\t') {
        line++;
    }
    char buf[sizeof(cmd->key) + sizeof(cmd->value) + 16];
    snprintf(buf, sizeof(buf), "%s", line);
    size_t n = strlen(buf);
    while (n > 0 && (buf[n - 1] == '\r' || buf[n - 1] == '\n' || buf[n - 1] == ' ')) {
        buf[--n] = '\0';
    }
    if (n == 0) {
        cmd->kind = DM_CMD_NONE;
        return;
    }
    char *rest = strchr(buf, ' ');
    if (rest != NULL) {
        *rest++ = '\0';
        while (*rest == ' ') {
            rest++;
        }
    }
    if (strcmp(buf, "help") == 0 || strcmp(buf, "?") == 0) {
        cmd->kind = DM_CMD_HELP;
    } else if (strcmp(buf, "show") == 0) {
        cmd->kind = DM_CMD_SHOW;
    } else if (strcmp(buf, "restart") == 0) {
        cmd->kind = DM_CMD_RESTART;
    } else if (strcmp(buf, "factory-reset") == 0) {
        cmd->kind = DM_CMD_FACTORY_RESET;
    } else if (strcmp(buf, "identify") == 0) {
        cmd->kind = DM_CMD_IDENTIFY;
    } else if (strcmp(buf, "set") == 0 && rest != NULL && *rest != '\0') {
        // set KEY [VALUE…] – the value is the rest of the line (may contain
        // spaces, e.g. a password); no value resets the key.
        cmd->kind = DM_CMD_SET;
        char *val = strchr(rest, ' ');
        if (val != NULL) {
            *val++ = '\0';
            while (*val == ' ') {
                val++;
            }
            snprintf(cmd->value, sizeof(cmd->value), "%s", val);
        }
        snprintf(cmd->key, sizeof(cmd->key), "%s", rest);
    } else {
        cmd->kind = DM_CMD_UNKNOWN;
    }
}
