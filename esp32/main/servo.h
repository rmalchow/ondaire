// servo.h — drift telemetry + (optional) rate servo (player-protocol.md §9, docs/developer/esp32.md
// §3.3). v1 measures the playout phase error and freezes a per-room device-delay
// estimate (D63) the master can diff for cross-room EQ (D65); APLL actuation is
// scaffolded but off by default (a clean glitch-free retune isn't available), so
// the node advertises queue=0 and relies on the skip/silence floor.
#pragma once
#include <stdbool.h>
#include <stdint.h>

void    servo_init(bool actuate);
void    servo_reset(void);                 // on attach / gen change
void    servo_update(int64_t phase_err_ns); // per emitted frame, from the player

int32_t servo_rate_ppm_x1000(void);        // STATUS ratePPMx1000 (0 if not actuating)
int64_t servo_phase_err_ns(void);          // STATUS phaseErrNs (smoothed live error)
int64_t servo_device_delay_ns(void);       // STATUS deviceDelayNs (frozen once calibrated)
bool    servo_calibrated(void);            // STATUS flag bit2
