// Time helpers shared by the firmware and the host tests.
#pragma once

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// dm_parse_rfc3339 parses a timestamp as the hub writes it (Go's
// time.RFC3339Nano), e.g. "2026-10-28T14:00:00+01:00" or
// "2026-10-28T13:25:03.123456789Z", into Unix seconds (fractions are
// dropped). Returns 0 on success, -1 on a malformed string.
int dm_parse_rfc3339(const char *s, int64_t *unix_seconds);

#ifdef __cplusplus
}
#endif
