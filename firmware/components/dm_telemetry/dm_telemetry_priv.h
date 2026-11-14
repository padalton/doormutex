// Shared between the files of this component only.
#pragma once

#include "driver/i2c_master.h"

// dm_power_i2c_bus returns the I²C bus dm_power_init created, or NULL.
i2c_master_bus_handle_t dm_power_i2c_bus(void);
