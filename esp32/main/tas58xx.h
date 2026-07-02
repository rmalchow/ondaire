#pragma once
#include <stdbool.h>

// Bring up a TI TAS5805M / TAS5825M I2S DSP class-D amp over I2C for plain stereo
// passthrough at unity (0 dB) gain — the app still attenuates in software
// (volume.c), so the amp runs at unity and the room/encoder volume works as usual.
// The chip's 7-bit address is the compiled board's DEF_DAC_I2C_ADDR (TAS5805M =
// 0x2D, TAS5825M = 0x4C); the register init is identical for both parts.
//
// en_pin is the device PDN/enable, driven with TI's power-up timing (LOW → 1 ms →
// HIGH → 5 ms) before the register writes, or -1 if not wired. Requires the I2S
// BCLK/LRCK to already be running — these parts are MCLK-less and derive their
// internal clock from BCK, so they stay silent until both the clocks and this init
// are present. Returns false (and logs) on any I2C failure; the node still runs.
bool tas58xx_init(int sda, int scl, int en_pin);
