#include "servo.h"
#include "wire.h"
#include "i2s_out.h"

// Nominal output latency = the actual I2S DMA depth (i2s_out.h), so the reported
// device delay tracks the real buffer instead of a stale constant. 8 x 240 frames
// @ 48 kHz = 40 ms. Used as the device-delay baseline until phase error refines it.
#define NOMINAL_DELAY_NS \
    ((int64_t)I2S_DMA_DESC_NUM * I2S_DMA_FRAME_NUM * 1000000000LL / WIRE_SAMPLE_RATE)
#define CALIB_FRAMES       500                      // ~10 s of stable playout
#define JITTER_OK_NS       (2 * 1000000)            // 2 ms RMS-ish threshold

#define SERVO_CLAMP_PPM    300.0f                   // bound the actuation
#define SERVO_SLEW_PPM     2.0f                      // max ppm change per frame (smooth ramp-in)

static bool    s_actuate;
static int64_t s_phase;       // EWMA of phase error (ns) — telemetry
static int64_t s_jitter;      // EWMA of |phase - s_phase| (ns)
static int64_t s_device_ns;   // frozen device-delay estimate once calibrated
static bool    s_calibrated;
static int     s_stable;      // consecutive low-jitter frames

static float   s_ratio_ppm;   // current actuation (slew-limited toward the drift), ppm
static float   s_acc;         // fractional insert/drop accumulator (sample-pairs)

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
    s_ratio_ppm = 0;
    s_acc = 0;
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
    (void)s_actuate;
}

void servo_set_drift_ppm(float ppm) {
    if (ppm >  SERVO_CLAMP_PPM) ppm =  SERVO_CLAMP_PPM;
    if (ppm < -SERVO_CLAMP_PPM) ppm = -SERVO_CLAMP_PPM;
    float d = ppm - s_ratio_ppm;            // slew toward the target (no overshoot)
    if (d >  SERVO_SLEW_PPM) d =  SERVO_SLEW_PPM;
    if (d < -SERVO_SLEW_PPM) d = -SERVO_SLEW_PPM;
    s_ratio_ppm += d;
}

int servo_sample_adjust(void) {
    // Accumulate the fractional samples this frame owes (ratio_ppm * frame samples);
    // emit a whole-sample insert (+1, fast crystal) / drop (-1) when it crosses ±1.
    s_acc += s_ratio_ppm * (float)WIRE_FRAME_SAMPLES * 1e-6f;
    if (s_acc >= 1.0f) { s_acc -= 1.0f; return +1; }
    if (s_acc <= -1.0f) { s_acc += 1.0f; return -1; }
    return 0;
}

int32_t servo_rate_ppm_x1000(void) { return (int32_t)(s_ratio_ppm * 1000.0f); }
int64_t servo_phase_err_ns(void)   { return s_phase; }
int64_t servo_device_delay_ns(void){ return s_device_ns; }
bool    servo_calibrated(void)     { return s_calibrated; }
