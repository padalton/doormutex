#include "check.h"
#include "dm_time.h"

void test_time(void)
{
    int64_t t = -1;
    CHECK_INT(dm_parse_rfc3339("1970-01-01T00:00:00Z", &t), 0);
    CHECK_INT(t, 0);

    // The hub's format: local time with offset.
    CHECK_INT(dm_parse_rfc3339("2026-10-28T14:00:00+01:00", &t), 0);
    CHECK_INT(t, 1793192400); // 2026-10-28T13:00:00Z

    // Go writes nanoseconds when they aren't zero.
    CHECK_INT(dm_parse_rfc3339("2026-10-28T14:25:03.123456789+01:00", &t), 0);
    CHECK_INT(t, 1793192400 + 25 * 60 + 3);

    CHECK_INT(dm_parse_rfc3339("2026-07-01T08:00:00-04:30", &t), 0);
    CHECK_INT(t, 1782909000); // 12:30Z

    // Leap day and century rules.
    CHECK_INT(dm_parse_rfc3339("2028-02-29T00:00:00Z", &t), 0);
    CHECK_INT(t, 1835395200);
    CHECK_INT(dm_parse_rfc3339("2100-02-29T00:00:00Z", &t), -1);
    CHECK_INT(dm_parse_rfc3339("2000-02-29T00:00:00Z", &t), 0);
    CHECK_INT(t, 951782400);

    // Malformed.
    const char *bad[] = {
        "", "2026-10-28", "2026-10-28T14:00:00", "2026-13-01T00:00:00Z", "2026-10-28T24:00:00Z",
        "2026-10-28T14:00:00+0100", "2026-10-28T14:00:00Zjunk", "2026-10-28T14:00:00.Z", "2026-1O-28T14:00:00Z",
    };
    for (size_t i = 0; i < sizeof(bad) / sizeof(bad[0]); i++) {
        CHECK_INT(dm_parse_rfc3339(bad[i], &t), -1);
    }
    CHECK_INT(dm_parse_rfc3339(NULL, &t), -1);
}
