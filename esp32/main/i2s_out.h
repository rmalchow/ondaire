// i2s_out.h — I2S standard-mode TX to the DAC. 48 kHz / stereo / 16-bit, APLL
// clock source for low jitter. PCM5102A runs MCLK-less (mclk = -1).
#pragma once
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

bool i2s_out_init(int bclk, int lrck, int dout, int mclk);

// Write one block of interleaved s16le stereo. Blocks on DMA backpressure (this
// is the playout pacing backstop). Returns bytes written.
size_t i2s_out_write(const int16_t *pcm, size_t bytes);

// Runtime sample-rate trim for a future APLL rate servo (disable→reconfig→enable;
// causes a brief gap, so unused by default — see servo.c / PLAYER §9).
bool i2s_out_set_rate_hz(uint32_t hz);

void i2s_out_stop(void);    // mute the line on detach (zero-fill + drop)
