# Pure logic without ESP-IDF APIs – also built on the host for the unit
# tests (firmware/test/host). Only cJSON is needed, which ESP-IDF ships.
idf_component_register(
    SRCS
        "src/dm_config.c"
        "src/dm_discovery.c"
        "src/dm_improv.c"
        "src/dm_pattern.c"
        "src/dm_state.c"
        "src/dm_time.c"
    INCLUDE_DIRS "include"
    REQUIRES json
)
