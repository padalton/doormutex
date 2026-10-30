idf_component_register(
    SRCS "dm_lights.c"
    INCLUDE_DIRS "include"
    REQUIRES dm_core esp_driver_ledc esp_timer
)
