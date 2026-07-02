#pragma once
#include <stdbool.h>

// Bring up an Infineon MA12070P I2S class-D DSP amp over I2C for plain stereo
// passthrough at unity (0 dB) gain — the app still attenuates in software
// (volume.c). The chip's 7-bit address is the compiled board's DEF_DAC_I2C_ADDR
// (0x20 as strapped on the Loud-ESP32-Plus).
//
// en_pin is the ACTIVE-LOW enable (driven LOW to run the part), or -1. The mute
// line is the board's DEF_MA_MUTE (LOW = muted, HIGH = unmuted), or -1 if not
// wired. Power-up order per the Infineon driver: mute LOW + enable LOW → wait
// ~100 ms for PVDD → write init → mute HIGH. Requires the I2S clocks to be running.
// Returns false (and logs) on any I2C failure; the node still runs.
bool ma12070p_init(int sda, int scl, int en_pin);
