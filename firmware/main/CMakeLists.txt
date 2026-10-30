idf_component_register(
    SRCS "main.c"
    INCLUDE_DIRS "."
    REQUIRES dm_core dm_lights dm_net dm_provision dm_telemetry dm_eink nvs_flash esp_pm app_update esp_timer
)
