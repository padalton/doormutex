#include <stdbool.h>
#include <stdint.h>
#include <string.h>

#include "battery_golden.h"
#include "check.h"
#include "dm_battery.h"

#define FRAME_BYTES (200 * 200 / 8)

// The light's battery must come out exactly like the hub's: the footers in
// battery_golden.h were rendered by the hub without and with the battery.
static void test_golden(void)
{
    static uint8_t frame[FRAME_BYTES];
    for (size_t i = 0; i < sizeof(battery_golden) / sizeof(battery_golden[0]); i++) {
        memset(frame, 0xff, sizeof(frame));
        frame[0] = 0x5a; // something above the footer that must stay
        memcpy(frame + GOLDEN_ROW0 * 25, battery_golden[i].base, GOLDEN_BYTES);
        dm_battery_draw(frame, battery_golden[i].percent, battery_golden[i].charging);
        int diff = -1;
        for (int j = 0; j < GOLDEN_BYTES; j++) {
            if (frame[GOLDEN_ROW0 * 25 + j] != battery_golden[i].want[j]) {
                diff = j;
                break;
            }
        }
        if (diff >= 0) {
            fprintf(stderr, "battery %d%%%s: first difference at row %d, byte %d\n", battery_golden[i].percent,
                    battery_golden[i].charging ? " charging" : "", GOLDEN_ROW0 + diff / 25, diff % 25);
        }
        CHECK_INT(diff, -1);
        CHECK_INT(frame[0], 0x5a);
    }

    // No gauge: the frame stays as the hub sent it.
    memset(frame, 0xff, sizeof(frame));
    memcpy(frame + GOLDEN_ROW0 * 25, battery_golden[0].base, GOLDEN_BYTES);
    dm_battery_draw(frame, -1, false);
    CHECK(memcmp(frame + GOLDEN_ROW0 * 25, battery_golden[0].base, GOLDEN_BYTES) == 0);
}

static void test_badge(void)
{
    dm_battery_badge_t b = {0};
    CHECK(!dm_battery_badge_update(&b, -1, false)); // no gauge, nothing shown
    CHECK(dm_battery_badge_update(&b, 80, false));  // appears
    CHECK_INT(b.percent, 80);
    CHECK(!dm_battery_badge_update(&b, 77, false)); // jitter below the step
    CHECK(!dm_battery_badge_update(&b, 84, false));
    CHECK(dm_battery_badge_update(&b, 75, false)); // a full step down
    CHECK_INT(b.percent, 75);
    CHECK(dm_battery_badge_update(&b, 75, true)); // charging starts
    CHECK(b.charging);
    CHECK(!dm_battery_badge_update(&b, 79, true));
    CHECK(dm_battery_badge_update(&b, 80, true));
    CHECK(dm_battery_badge_update(&b, 120, false)); // charging done, clamped
    CHECK_INT(b.percent, 100);
    CHECK(dm_battery_badge_update(&b, -1, false)); // gauge gone: disappears
    CHECK(!b.shown);
    CHECK(!dm_battery_badge_update(&b, -1, false));
}

void test_battery(void)
{
    test_golden();
    test_badge();
}
