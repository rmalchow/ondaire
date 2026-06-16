// i2s_out.h — I2S standard-mode TX to the DAC. 48 kHz / stereo / 16-bit, APLL
// clock source for low jitter. PCM5102A runs MCLK-less (mclk = -1).
#pragma once
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

// I2S DMA queue sizing. This IS the node's nominal output latency: at 48 kHz,
// 8 x 240 = 1920 frames = 40 ms (a touch above the ALSA nodes' ~36 ms, for
// underrun headroom). servo.c derives the device-delay baseline from these, so
// telemetry/equalization stay correct if the DMA depth changes.
#define I2S_DMA_DESC_NUM   8
#define I2S_DMA_FRAME_NUM  240   // frames per DMA descriptor

bool i2s_out_init(int bclk, int lrck, int dout, int mclk);

// Write one block of interleaved s16le stereo. Blocks on DMA backpressure (this
// is the playout pacing backstop). Returns bytes written.
size_t i2s_out_write(const int16_t *pcm, size_t bytes);

// Runtime sample-rate trim for a future APLL rate servo (disable→reconfig→enable;
// causes a brief gap, so unused by default — see servo.c / PLAYER §9).
bool i2s_out_set_rate_hz(uint32_t hz);

void i2s_out_stop(void);    // mute the line on detach (zero-fill + drop)
