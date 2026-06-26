// servo.h — playout rate matching (player-protocol.md §9, docs/developer/esp32.md §3.3).
//
// The S3's I2S has no APLL, so we can't trim the DAC clock; instead the master-
// paced playout writes a slightly long/short frame now and then so its long-run
// output rate equals the master's. The actuation is FEED-FORWARD from the measured
// crystal drift (clock_drift_ppm_x1000) — NOT a phase feedback loop — so it can't
// rail on the clock's noisy millisecond-scale offset steps. servo_update() still
// tracks the phase error for telemetry + the frozen device-delay estimate (D63).
#pragma once
#include <stdbool.h>
#include <stdint.h>

void    servo_init(bool actuate);
void    servo_reset(void);                  // on attach / gen change / re-prime
void    servo_update(int64_t phase_err_ns); // per frame: phase telemetry + calibration

// Feed-forward actuation: the player passes the measured crystal drift (ppm,
// local-fast positive) each frame; the servo slew-limits + clamps it.
void    servo_set_drift_ppm(float ppm);
// Per-frame sample adjustment from the accumulated drift: +1 = insert one extra
// sample-pair (fast crystal), -1 = drop one, 0 = pass-through.
int     servo_sample_adjust(void);

int32_t servo_rate_ppm_x1000(void);         // STATUS ratePPMx1000 (live actuation)
int64_t servo_phase_err_ns(void);           // STATUS phaseErrNs (smoothed live error)
int64_t servo_device_delay_ns(void);        // STATUS deviceDelayNs (frozen once calibrated)
bool    servo_calibrated(void);             // STATUS flag bit2
