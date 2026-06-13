#include "volume.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"

static SemaphoreHandle_t mu;
static uint8_t s_pct = 60;
static bool    s_mute;
static int32_t s_gain_q15 = 1 << 15;   // current applied gain (Q15)

// Perceptual curve: gain = (pct/100)^3, so the knob feels even across its range.
static int32_t curve_q15(uint8_t pct) {
    if (pct == 0) return 0;
    if (pct > 100) pct = 100;
    // (pct^3 / 100^3) in Q15 without floats.
    int64_t num = (int64_t)pct * pct * pct;     // up to 1e6
    return (int32_t)((num << 15) / 1000000);
}

static void recompute(void) {
    s_gain_q15 = s_mute ? 0 : curve_q15(s_pct);
}

void volume_init(uint8_t pct) {
    mu = xSemaphoreCreateMutex();
    s_pct = pct > 100 ? 100 : pct;
    s_mute = false;
    recompute();
}

void volume_set(uint8_t pct, bool mute) {
    xSemaphoreTake(mu, portMAX_DELAY);
    s_pct = pct > 100 ? 100 : pct;
    s_mute = mute;
    recompute();
    xSemaphoreGive(mu);
}

uint8_t volume_get(void)  { return s_pct; }
bool    volume_muted(void) { return s_mute; }

void volume_apply(int16_t *pcm, size_t nsamples) {
    int32_t g = s_gain_q15;          // atomic-enough 32-bit read
    if (g == (1 << 15)) return;      // unity: nothing to do
    if (g == 0) {
        for (size_t i = 0; i < nsamples; i++) pcm[i] = 0;
        return;
    }
    for (size_t i = 0; i < nsamples; i++) {
        int32_t v = ((int32_t)pcm[i] * g) >> 15;
        if (v > 32767) v = 32767; else if (v < -32768) v = -32768;
        pcm[i] = (int16_t)v;
    }
}
