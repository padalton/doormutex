#include "dm_battery.h"

#include <stdlib.h>

#include "dm_battery_font.h"

#define W 200
#define H 200
#define STRIDE (W / 8)

static void pixel(uint8_t *frame, int x, int y, bool black)
{
    uint8_t bit = (uint8_t)(0x80 >> (x % 8));
    if (black) {
        frame[y * STRIDE + x / 8] &= (uint8_t)~bit;
    } else {
        frame[y * STRIDE + x / 8] |= bit;
    }
}

// rect fills [x0, x1) × [y0, y1) like the hub's canvas.rect.
static void rect(uint8_t *frame, int x0, int y0, int x1, int y1, bool black)
{
    for (int y = y0; y < y1; y++) {
        for (int x = x0; x < x1; x++) {
            pixel(frame, x, y, black);
        }
    }
}

void dm_battery_draw(uint8_t *frame, int percent, bool charging)
{
    if (percent < 0) {
        return;
    }
    if (percent > 100) {
        percent = 100;
    }
    // Clear the corner: from the widest label to the symbol's tip, below the rule.
    for (int y = BAT_RULE_Y + 1; y < H; y++) {
        for (int b = BAT_LABEL_BYTE0; b <= (BAT_X1 + 2) / 8; b++) {
            frame[y * STRIDE + b] = 0xff;
        }
    }
    // The hub draws the rule whenever the footer has something in it.
    rect(frame, BAT_RULE_X0, BAT_RULE_Y, BAT_RULE_X1, BAT_RULE_Y + 1, true);

    rect(frame, BAT_X0, BAT_Y0, BAT_X1, BAT_Y1, true);
    rect(frame, BAT_X0 + 2, BAT_Y0 + 2, BAT_X1 - 2, BAT_Y1 - 2, false);
    rect(frame, BAT_X1, BAT_Y0 + 3, BAT_X1 + 3, BAT_Y1 - 3, true);
    int fill = (BAT_X1 - BAT_X0 - 6) * percent / 100;
    if (fill > 0) {
        rect(frame, BAT_X0 + 3, BAT_Y0 + 3, BAT_X0 + 3 + fill, BAT_Y1 - 3, true);
    }
    if (charging) {
        for (int r = 0; r < BAT_BOLT_ROWS; r++) {
            for (int b = 0; b < BAT_BOLT_BYTES; b++) {
                frame[(BAT_BOLT_ROW0 + r) * STRIDE + BAT_BOLT_BYTE0 + b] |= bat_bolt[r * BAT_BOLT_BYTES + b];
            }
        }
    }
    const uint8_t *label = bat_labels[percent];
    for (int r = 0; r < BAT_LABEL_ROWS; r++) {
        for (int b = 0; b < BAT_LABEL_BYTES; b++) {
            frame[(BAT_LABEL_ROW0 + r) * STRIDE + BAT_LABEL_BYTE0 + b] &= (uint8_t)~label[r * BAT_LABEL_BYTES + b];
        }
    }
}

bool dm_battery_badge_update(dm_battery_badge_t *b, int percent, bool charging)
{
    if (percent > 100) {
        percent = 100;
    }
    if (percent < 0) {
        bool was = b->shown;
        b->shown = false;
        return was;
    }
    if (b->shown && charging == b->charging && abs(percent - b->percent) < DM_BATTERY_STEP) {
        return false;
    }
    b->shown = true;
    b->percent = percent;
    b->charging = charging;
    return true;
}
