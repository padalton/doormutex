#include "check.h"

int dm_failures;
int dm_checks;

int main(void)
{
    struct {
        const char *name;
        void (*fn)(void);
    } suites[] = {
        {"time", test_time},       {"state", test_state},   {"failsafe", test_failsafe},
        {"pattern", test_pattern}, {"improv", test_improv}, {"config", test_config},
        {"discovery", test_discovery},
    };
    for (size_t i = 0; i < sizeof(suites) / sizeof(suites[0]); i++) {
        int before = dm_failures;
        suites[i].fn();
        printf("%-10s %s\n", suites[i].name, dm_failures == before ? "ok" : "FAIL");
    }
    printf("%d checks, %d failures\n", dm_checks, dm_failures);
    return dm_failures == 0 ? 0 : 1;
}
