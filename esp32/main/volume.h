// volume.h — software volume for the PCM5102A (no hardware volume register).
// A perceptual (≈cubic) curve maps 0..100 % to a Q15 gain applied in-place to
// s16 samples just before I2S. Shared by the rotary knob and master SETVOL.
#pragma once
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

void    volume_init(uint8_t pct);
void    volume_set(uint8_t pct, bool mute);   // 0..100; clamps
uint8_t volume_get(void);
bool    volume_muted(void);

// Apply the current gain to nsamples interleaved s16 values, in place.
void    volume_apply(int16_t *pcm, size_t nsamples);
