// A tiny test runner: CHECK macros count failures, main.c runs all suites.
#pragma once

#include <stdio.h>
#include <string.h>

extern int dm_failures;
extern int dm_checks;

#define CHECK(cond)                                                          \
    do {                                                                     \
        dm_checks++;                                                         \
        if (!(cond)) {                                                       \
            dm_failures++;                                                   \
            fprintf(stderr, "%s:%d: CHECK(%s) failed\n", __FILE__, __LINE__, #cond); \
        }                                                                    \
    } while (0)

#define CHECK_INT(got, want)                                                 \
    do {                                                                     \
        long long g_ = (long long)(got), w_ = (long long)(want);             \
        dm_checks++;                                                         \
        if (g_ != w_) {                                                      \
            dm_failures++;                                                   \
            fprintf(stderr, "%s:%d: %s = %lld, want %lld\n", __FILE__, __LINE__, #got, g_, w_); \
        }                                                                    \
    } while (0)

#define CHECK_STR(got, want)                                                 \
    do {                                                                     \
        const char *g_ = (got), *w_ = (want);                                \
        dm_checks++;                                                         \
        if (g_ == NULL || strcmp(g_, w_) != 0) {                             \
            dm_failures++;                                                   \
            fprintf(stderr, "%s:%d: %s = \"%s\", want \"%s\"\n", __FILE__, __LINE__, #got, g_ ? g_ : "(null)", w_); \
        }                                                                    \
    } while (0)

void test_time(void);
void test_state(void);
void test_failsafe(void);
void test_pattern(void);
void test_improv(void);
void test_config(void);
void test_discovery(void);
