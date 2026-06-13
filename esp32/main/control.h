// control.h — the v2 control plane (PLAYER §6, mirrors
// internal/playback/control.go). Reads master→playback commands on CONTROL_PORT,
// applies them idempotently (soft-state, re-asserted on a ~1 Hz heartbeat), and
// emits STATUS back to the master's source endpoint.
#pragma once
#include <stdbool.h>
#include <stdint.h>

bool control_init(uint16_t control_port);   // bind CONTROL_PORT, spawn tasks
