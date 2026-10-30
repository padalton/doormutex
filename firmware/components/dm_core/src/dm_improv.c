#include "dm_improv.h"

#include <string.h>

static const char header[] = "IMPROV";
#define HEADER_LEN 6

void dm_improv_init(dm_improv_parser_t *p)
{
    memset(p, 0, sizeof(*p));
}

dm_improv_feed_t dm_improv_feed(dm_improv_parser_t *p, uint8_t b, dm_improv_frame_t *frame)
{
    dm_improv_feed_t r = {.result = DM_IMPROV_MORE};
    if (p->pos < HEADER_LEN) {
        if (b == (uint8_t)header[p->pos]) {
            p->sum += b;
            p->pos++;
            return r;
        }
        // Not a frame after all: the header bytes seen so far are text.
        r.result = DM_IMPROV_TEXT;
        r.text_prefix = (uint8_t)p->pos;
        dm_improv_init(p);
        if (b == (uint8_t)header[0]) {
            p->sum = b; // this byte may start a real frame
            p->pos = 1;
        } else {
            r.byte_is_text = true;
        }
        return r;
    }
    uint16_t at = p->pos - HEADER_LEN;
    if (at == 0) {
        p->version = b;
    } else if (at == 1) {
        p->type = b;
    } else if (at == 2) {
        p->len = b;
    } else if (at - 3 < p->len) {
        p->data[at - 3] = b;
    } else {
        // checksum byte
        bool ok = b == p->sum;
        if (ok) {
            frame->type = p->type;
            frame->len = p->len;
            memcpy(frame->data, p->data, p->len);
        }
        dm_improv_init(p);
        r.result = ok ? DM_IMPROV_FRAME : DM_IMPROV_BAD_CHECKSUM;
        return r;
    }
    p->sum += b;
    p->pos++;
    return r;
}

// take_string reads a length-prefixed string from data[*i…].
static int take_string(const uint8_t *data, uint8_t end, uint8_t *i, char *dst, size_t cap)
{
    if (*i >= end) {
        return -1;
    }
    uint8_t n = data[(*i)++];
    if (n >= cap || *i + n > end) {
        return -1;
    }
    memcpy(dst, data + *i, n);
    dst[n] = '\0';
    *i += n;
    return 0;
}

int dm_improv_parse_rpc(const dm_improv_frame_t *frame, dm_improv_rpc_t *rpc)
{
    memset(rpc, 0, sizeof(*rpc));
    if (frame->type != DM_IMPROV_TYPE_RPC || frame->len < 2) {
        return -1;
    }
    rpc->command = frame->data[0];
    uint8_t dlen = frame->data[1];
    if (2 + dlen > frame->len) {
        return -1;
    }
    if (rpc->command == DM_IMPROV_CMD_WIFI_SETTINGS) {
        uint8_t i = 2, end = (uint8_t)(2 + dlen);
        if (take_string(frame->data, end, &i, rpc->ssid, sizeof(rpc->ssid)) != 0 ||
            take_string(frame->data, end, &i, rpc->password, sizeof(rpc->password)) != 0 || rpc->ssid[0] == '\0') {
            return -1;
        }
    }
    return 0;
}

size_t dm_improv_build(uint8_t type, const uint8_t *data, uint8_t len, uint8_t *out, size_t cap)
{
    size_t total = HEADER_LEN + 3 + (size_t)len + 1 + 1;
    if (total > cap) {
        return 0;
    }
    memcpy(out, header, HEADER_LEN);
    out[6] = DM_IMPROV_VERSION;
    out[7] = type;
    out[8] = len;
    if (len > 0) {
        memcpy(out + 9, data, len);
    }
    uint8_t sum = 0;
    for (size_t i = 0; i < 9 + (size_t)len; i++) {
        sum += out[i];
    }
    out[9 + len] = sum;
    out[10 + len] = '\n';
    return total;
}

size_t dm_improv_build_state(dm_improv_state_t state, uint8_t *out, size_t cap)
{
    uint8_t b = (uint8_t)state;
    return dm_improv_build(DM_IMPROV_TYPE_CURRENT_STATE, &b, 1, out, cap);
}

size_t dm_improv_build_error(dm_improv_error_t err, uint8_t *out, size_t cap)
{
    uint8_t b = (uint8_t)err;
    return dm_improv_build(DM_IMPROV_TYPE_ERROR_STATE, &b, 1, out, cap);
}

size_t dm_improv_build_result(uint8_t command, const char *const *strings, size_t n, uint8_t *out, size_t cap)
{
    uint8_t data[255];
    size_t len = 2;
    for (size_t i = 0; i < n; i++) {
        size_t sl = strlen(strings[i]);
        if (sl > 255 || len + 1 + sl > sizeof(data)) {
            return 0;
        }
        data[len++] = (uint8_t)sl;
        memcpy(data + len, strings[i], sl);
        len += sl;
    }
    data[0] = command;
    data[1] = (uint8_t)(len - 2);
    return dm_improv_build(DM_IMPROV_TYPE_RPC_RESULT, data, (uint8_t)len, out, cap);
}
