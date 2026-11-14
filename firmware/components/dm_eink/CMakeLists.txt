idf_component_register(
    SRCS "dm_eink.c"
    INCLUDE_DIRS "include"
    REQUIRES esp_rom esp_driver_gpio esp_driver_spi esp_timer
)
