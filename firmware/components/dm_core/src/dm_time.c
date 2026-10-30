#include "dm_time.h"

#include <stdbool.h>
#include <stddef.h>

// digits reads n decimal digits.
static bool digits(const char *s, int n, int *out)
{
    int v = 0;
    for (int i = 0; i < n; i++) {
        if (s[i] < '0' || s[i] > '9') {
            return false;
        }
        v = v * 10 + (s[i] - '0');
    }
    *out = v;
    return true;
}

// days_from_civil returns the days since 1970-01-01 of a proleptic
// Gregorian date (Howard Hinnant's algorithm).
static int64_t days_from_civil(int64_t y, int m, int d)
{
    y -= m <= 2;
    const int64_t era = (y >= 0 ? y : y - 399) / 400;
    const int64_t yoe = y - era * 400;
    const int64_t doy = (153 * (m > 2 ? m - 3 : m + 9) + 2) / 5 + d - 1;
    const int64_t doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    return era * 146097 + doe - 719468;
}

static int days_in_month(int y, int m)
{
    static const int dm[] = {31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31};
    if (m == 2 && ((y % 4 == 0 && y % 100 != 0) || y % 400 == 0)) {
        return 29;
    }
    return dm[m - 1];
}

int dm_parse_rfc3339(const char *s, int64_t *unix_seconds)
{
    int y, mo, d, h, mi, sec;
    if (s == NULL || unix_seconds == NULL) {
        return -1;
    }
    // YYYY-MM-DDTHH:MM:SS – checked character by character, so a short
    // string stops at its terminating NUL.
    if (!digits(s, 4, &y) || s[4] != '-' || !digits(s + 5, 2, &mo) || s[7] != '-' ||
        !digits(s + 8, 2, &d) || (s[10] != 'T' && s[10] != 't' && s[10] != ' ') ||
        !digits(s + 11, 2, &h) || s[13] != ':' || !digits(s + 14, 2, &mi) || s[16] != ':' ||
        !digits(s + 17, 2, &sec)) {
        return -1;
    }
    if (mo < 1 || mo > 12 || d < 1 || d > days_in_month(y, mo) || h > 23 || mi > 59 || sec > 60) {
        return -1;
    }
    const char *p = s + 19;
    if (*p == '.') {
        p++;
        if (*p < '0' || *p > '9') {
            return -1;
        }
        while (*p >= '0' && *p <= '9') {
            p++;
        }
    }
    int offset = 0;
    if (*p == 'Z' || *p == 'z') {
        p++;
    } else if (*p == '+' || *p == '-') {
        int oh, om;
        if (!digits(p + 1, 2, &oh) || p[3] != ':' || !digits(p + 4, 2, &om) || oh > 23 || om > 59) {
            return -1;
        }
        offset = (oh * 60 + om) * 60;
        if (*p == '-') {
            offset = -offset;
        }
        p += 6;
    } else {
        return -1;
    }
    if (*p != '\0') {
        return -1;
    }
    *unix_seconds = days_from_civil(y, mo, d) * 86400 + h * 3600 + mi * 60 + sec - offset;
    return 0;
}
