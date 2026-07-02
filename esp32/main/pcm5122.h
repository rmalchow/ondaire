// pcm5122.h — minimal I2C bring-up for a TI PCM5122 DAC strapped in software
// (I2C) control mode, running MCLK-less (system clock derived from BCK by the
// chip's PLL). Unlike the PCM5102A (hardware-config pins, plays on power-up),
// the PCM5122 in I2C mode does not emit any analog output until its PLL is told
// to reference BCK — so a MCLK-less board is silent (line-out AND amp) until
// this init runs. Used by the Sonocotta Amped-ESP32-S3-Plus (dac=1).
#pragma once
#include <stdbool.h>

// Bring up I2C on the given pins and write the minimal PCM5122 init sequence
// (PLL reference = BCK, un-standby, un-mute, 0 dB digital volume) to the DAC's
// I2C address (DEF_DAC_I2C_ADDR for the compiled board). Requires the I2S clocks
// to already be running so the DAC's PLL can lock. en_pin is the DAC hard-enable
// (driven HIGH before the I2C writes, e.g. HiFi-ESP32-Plus GPIO4), or -1 if the
// board has none (e.g. Amped-ESP32-S3-Plus, rev C "no DAC_EN"). Returns false if
// the device does not ACK or a register write fails; logs the reason.
bool pcm5122_init(int sda, int scl, int en_pin);
