// Improv Wi-Fi serial protocol (https://www.improv-wifi.com/serial/), as
// spoken by ESP Web Tools: after flashing in the browser it hands over the
// Wi-Fi credentials over the same USB connection.
//
// Frame: "IMPROV" | version (1) | type | length | data… | checksum
// (checksum = sum of all previous bytes, mod 256). The firmware appends a
// newline so frames don't glue onto log lines.
//
// The serial line also carries log output and a small text console, so
// the parser is fed byte by byte and tells the caller which bytes were not
// part of a frame.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define DM_IMPROV_VERSION 1

typedef enum {
    DM_IMPROV_TYPE_CURRENT_STATE = 0x01,
    DM_IMPROV_TYPE_ERROR_STATE = 0x02,
    DM_IMPROV_TYPE_RPC = 0x03,
    DM_IMPROV_TYPE_RPC_RESULT = 0x04,
} dm_improv_type_t;

typedef enum {
    DM_IMPROV_STATE_READY = 0x02, // "authorized": ready to accept credentials
    DM_IMPROV_STATE_PROVISIONING = 0x03,
    DM_IMPROV_STATE_PROVISIONED = 0x04,
} dm_improv_state_t;

typedef enum {
    DM_IMPROV_ERROR_NONE = 0x00,
    DM_IMPROV_ERROR_INVALID_RPC = 0x01,
    DM_IMPROV_ERROR_UNKNOWN_RPC = 0x02,
    DM_IMPROV_ERROR_UNABLE_TO_CONNECT = 0x03,
    DM_IMPROV_ERROR_UNKNOWN = 0xFF,
} dm_improv_error_t;

typedef enum {
    DM_IMPROV_CMD_WIFI_SETTINGS = 0x01,
    DM_IMPROV_CMD_GET_CURRENT_STATE = 0x02,
    DM_IMPROV_CMD_GET_DEVICE_INFO = 0x03,
    DM_IMPROV_CMD_GET_WIFI_NETWORKS = 0x04,
} dm_improv_cmd_t;

typedef struct {
    uint8_t type;
    uint8_t len;
    uint8_t data[255];
} dm_improv_frame_t;

typedef enum {
    DM_IMPROV_MORE = 0,      // byte consumed, frame not complete yet
    DM_IMPROV_FRAME,         // *frame holds a complete, valid frame
    DM_IMPROV_TEXT,          // not a frame: see dm_improv_feed_result_t
    DM_IMPROV_BAD_CHECKSUM,  // a complete frame with a wrong checksum (dropped)
} dm_improv_result_t;

typedef struct {
    uint16_t pos; // bytes of the current frame received so far (≤ 6 + 3 + 255)
    uint8_t version, type, len;
    uint8_t sum;
    uint8_t data[255];
} dm_improv_parser_t;

typedef struct {
    dm_improv_result_t result;
    // For DM_IMPROV_TEXT: the first text_prefix bytes of "IMPROV" turned
    // out not to start a frame and are text, followed by the current byte
    // if byte_is_text.
    uint8_t text_prefix;
    bool byte_is_text;
} dm_improv_feed_t;

void dm_improv_init(dm_improv_parser_t *p);
dm_improv_feed_t dm_improv_feed(dm_improv_parser_t *p, uint8_t byte, dm_improv_frame_t *frame);

// RPC parsing. Returns 0 on success, -1 on a malformed RPC.
typedef struct {
    uint8_t command;
    char ssid[33];
    char password[65];
} dm_improv_rpc_t;

int dm_improv_parse_rpc(const dm_improv_frame_t *frame, dm_improv_rpc_t *rpc);

// Frame builders: they write the frame (with checksum and newline) into
// out and return its length, or 0 if it doesn't fit.
size_t dm_improv_build(uint8_t type, const uint8_t *data, uint8_t len, uint8_t *out, size_t cap);
size_t dm_improv_build_state(dm_improv_state_t state, uint8_t *out, size_t cap);
size_t dm_improv_build_error(dm_improv_error_t err, uint8_t *out, size_t cap);
// An RPC result: the command followed by a list of strings.
size_t dm_improv_build_result(uint8_t command, const char *const *strings, size_t n, uint8_t *out, size_t cap);

#ifdef __cplusplus
}
#endif
