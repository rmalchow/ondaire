#include "servo.h"
#include "wire.h"
#include "i2s_out.h"

#include <stdlib.h>

// Nominal output latency = the actual I2S DMA depth (i2s_out.h), so the reported
// device delay tracks the real buffer instead of a stale constant. 6 x 240 frames
// @ 48 kHz = 30 ms. Used as the device-delay baseline until phase error refines it.
#define NOMINAL_DELAY_NS \
    ((int64_t)I2S_DMA_DESC_NUM * I2S_DMA_FRAME_NUM * 1000000000LL / WIRE_SAMPLE_RATE)
#define CALIB_FRAMES       500                      // ~10 s of stable playout
#define JITTER_OK_NS       (2 * 1000000)            // 2 ms RMS-ish threshold

static bool    s_actuate;
static int64_t s_phase;       // EWMA of phase error (ns)
static int64_t s_jitter;      // EWMA of |phase - s_phase| (ns)
static int64_t s_device_ns;   // frozen device-delay estimate once calibrated
static bool    s_calibrated;
static int     s_stable;      // consecutive low-jitter frames

void servo_init(bool actuate) {
    s_actuate = actuate;
    servo_reset();
}

void servo_reset(void) {
    s_phase = 0;
    s_jitter = 0;
    s_device_ns = NOMINAL_DELAY_NS;
    s_calibrated = false;
    s_stable = 0;
}

void servo_update(int64_t phase_err_ns) {
    // EWMA smoothing (alpha ~ 1/64) — heavy, to reject Wi-Fi scheduling jitter.
    s_phase += (phase_err_ns - s_phase) >> 6;
    int64_t dev = phase_err_ns - s_phase;
    if (dev < 0) dev = -dev;
    s_jitter += (dev - s_jitter) >> 6;

    if (!s_calibrated) {
        if (s_jitter < JITTER_OK_NS) {
            if (++s_stable >= CALIB_FRAMES) {
                s_device_ns = NOMINAL_DELAY_NS + s_phase;  // freeze per-room constant
                s_calibrated = true;
            }
        } else {
            s_stable = 0;
        }
    }
    // v1: no APLL actuation (s_actuate stays false on these boards); the
    // skip/silence floor bounds drift within bufferMs. ratePPMx1000 stays 0.
    (void)s_actuate;
}

int32_t servo_rate_ppm_x1000(void) { return 0; }
int64_t servo_phase_err_ns(void)   { return s_phase; }
int64_t servo_device_delay_ns(void){ return s_device_ns; }
bool    servo_calibrated(void)     { return s_calibrated; }
