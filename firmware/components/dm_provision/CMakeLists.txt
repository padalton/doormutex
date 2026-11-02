idf_component_register(
    SRCS "dm_provision.c"
    INCLUDE_DIRS "include"
    REQUIRES dm_core dm_net nvs_flash esp_driver_usb_serial_jtag esp_wifi vfs
)
