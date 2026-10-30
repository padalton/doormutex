idf_component_register(
    SRCS "dm_net.c"
    INCLUDE_DIRS "include"
    REQUIRES dm_core esp_wifi esp_netif esp_event esp_timer mqtt lwip mbedtls
)
