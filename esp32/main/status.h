// status.h — build the 87-byte STATUS telemetry payload (PLAYER §6.3).
#pragma once
#include <stddef.h>
#include <stdint.h>

int status_build(uint8_t *out, size_t cap);   // -> WIRE_STATUS_LEN, or -1
