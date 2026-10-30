#include <stdint.h>

#include "check.h"
#include "dm_improv.h"

// feed_all pushes bytes through the parser and collects text and frames.
static int feed_all(dm_improv_parser_t *p, const uint8_t *in, size_t n, char *text, size_t text_cap,
                    dm_improv_frame_t *frame, int *bad)
{
    int frames = 0;
    size_t tl = 0;
    for (size_t i = 0; i < n; i++) {
        dm_improv_feed_t r = dm_improv_feed(p, in[i], frame);
        if (r.result == DM_IMPROV_TEXT) {
            for (uint8_t k = 0; k < r.text_prefix && tl + 1 < text_cap; k++) {
                text[tl++] = "IMPROV"[k];
            }
            if (r.byte_is_text && tl + 1 < text_cap) {
                text[tl++] = (char)in[i];
            }
        } else if (r.result == DM_IMPROV_FRAME) {
            frames++;
        } else if (r.result == DM_IMPROV_BAD_CHECKSUM) {
            (*bad)++;
        }
    }
    text[tl] = '\0';
    return frames;
}

void test_improv(void)
{
    uint8_t buf[300];
    // Build a Wi-Fi settings RPC as ESP Web Tools sends it.
    const char *ssid = "Home Net", *pass = "s3cret pass";
    uint8_t data[100];
    size_t sl = strlen(ssid), pl = strlen(pass);
    data[0] = DM_IMPROV_CMD_WIFI_SETTINGS;
    data[1] = (uint8_t)(2 + sl + pl);
    data[2] = (uint8_t)sl;
    memcpy(data + 3, ssid, sl);
    data[3 + sl] = (uint8_t)pl;
    memcpy(data + 4 + sl, pass, pl);
    size_t n = dm_improv_build(DM_IMPROV_TYPE_RPC, data, (uint8_t)(4 + sl + pl), buf, sizeof(buf));
    CHECK_INT(n, 6 + 3 + 4 + sl + pl + 2);
    CHECK(memcmp(buf, "IMPROV\x01\x03", 8) == 0);
    CHECK_INT(buf[n - 1], '\n');

    // Surrounded by log text, with a false start ("IMP") in it.
    uint8_t stream[400];
    size_t sn = 0;
    const char *pre = "boot ok\nIMPORTANT\n";
    memcpy(stream, pre, strlen(pre));
    sn += strlen(pre);
    memcpy(stream + sn, buf, n - 1); // without the newline
    sn += n - 1;
    memcpy(stream + sn, "tail", 4);
    sn += 4;

    dm_improv_parser_t p;
    dm_improv_init(&p);
    dm_improv_frame_t frame;
    char text[200];
    int bad = 0;
    CHECK_INT(feed_all(&p, stream, sn, text, sizeof(text), &frame, &bad), 1);
    CHECK_STR(text, "boot ok\nIMPORTANT\ntail");
    CHECK_INT(bad, 0);

    dm_improv_rpc_t rpc;
    CHECK_INT(dm_improv_parse_rpc(&frame, &rpc), 0);
    CHECK_INT(rpc.command, DM_IMPROV_CMD_WIFI_SETTINGS);
    CHECK_STR(rpc.ssid, "Home Net");
    CHECK_STR(rpc.password, "s3cret pass");

    // "IMPROIMPROV…": the restart inside a false header still finds the frame.
    uint8_t s2[300];
    memcpy(s2, "IMPRO", 5);
    memcpy(s2 + 5, buf, n);
    dm_improv_init(&p);
    bad = 0;
    CHECK_INT(feed_all(&p, s2, 5 + n, text, sizeof(text), &frame, &bad), 1);
    CHECK_STR(text, "IMPRO\n");

    // A corrupted checksum is dropped.
    buf[n - 2] ^= 0xFF;
    dm_improv_init(&p);
    bad = 0;
    CHECK_INT(feed_all(&p, buf, n, text, sizeof(text), &frame, &bad), 0);
    CHECK_INT(bad, 1);

    // Malformed RPCs.
    dm_improv_frame_t f = {.type = DM_IMPROV_TYPE_RPC, .len = 4, .data = {DM_IMPROV_CMD_WIFI_SETTINGS, 2, 5, 'a'}};
    CHECK_INT(dm_improv_parse_rpc(&f, &rpc), -1); // ssid length beyond the data
    dm_improv_frame_t empty = {.type = DM_IMPROV_TYPE_RPC, .len = 4, .data = {DM_IMPROV_CMD_WIFI_SETTINGS, 2, 0, 0}};
    CHECK_INT(dm_improv_parse_rpc(&empty, &rpc), -1); // empty SSID
    dm_improv_frame_t info = {.type = DM_IMPROV_TYPE_RPC, .len = 2, .data = {DM_IMPROV_CMD_GET_DEVICE_INFO, 0}};
    CHECK_INT(dm_improv_parse_rpc(&info, &rpc), 0);
    CHECK_INT(rpc.command, DM_IMPROV_CMD_GET_DEVICE_INFO);

    // Results and states parse back with the parser itself.
    const char *strs[] = {"doormutex", "0.1.0", "ESP32-C6", "doormutex-a1b2c3"};
    n = dm_improv_build_result(DM_IMPROV_CMD_GET_DEVICE_INFO, strs, 4, buf, sizeof(buf));
    CHECK(n > 0);
    dm_improv_init(&p);
    bad = 0;
    CHECK_INT(feed_all(&p, buf, n, text, sizeof(text), &frame, &bad), 1);
    CHECK_INT(frame.type, DM_IMPROV_TYPE_RPC_RESULT);
    CHECK_INT(frame.data[0], DM_IMPROV_CMD_GET_DEVICE_INFO);
    CHECK_INT(frame.data[1], frame.len - 2);
    CHECK_INT(frame.data[2], 9);
    CHECK(memcmp(frame.data + 3, "doormutex", 9) == 0);

    n = dm_improv_build_state(DM_IMPROV_STATE_PROVISIONED, buf, sizeof(buf));
    CHECK_INT(n, 12);
    CHECK_INT(buf[8], 1);
    CHECK_INT(buf[9], DM_IMPROV_STATE_PROVISIONED);
    n = dm_improv_build_error(DM_IMPROV_ERROR_UNABLE_TO_CONNECT, buf, sizeof(buf));
    CHECK_INT(buf[7], DM_IMPROV_TYPE_ERROR_STATE);
    CHECK_INT(buf[9], DM_IMPROV_ERROR_UNABLE_TO_CONNECT);
    CHECK_INT(dm_improv_build_state(DM_IMPROV_STATE_READY, buf, 5), 0); // too small
}
