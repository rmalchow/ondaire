// Sonocotta Loud-ESP32-S3 — two Maxim MAX98357A mono I2S DAC+amp chips (one strapped
// Left, one Right) on the "audio dock" carrier, 2x5 W. Each MAX98357A is a
// self-contained I2S DAC + class-D amp: MCLK-less, no I2C, gain and L/R channel
// strapped in hardware. So this is the plain dac=0 path — no DAC init — and the
// shared SD/enable line (GPIO17) is the amp gate. Sonocotta's ESPHome restores that
// pin OFF at boot (dac_enable_restore_mode: ALWAYS_OFF); we get the same behaviour
// from player.c's amp_en idle-gate (starts LOW, HIGH only while audio plays).
// Pin map from github.com/sonocotta/esp32-audio-dock, firmware/esphome/
// 2-loud-esp32/loud-esp32-s3-idf.yaml.
//
// Chip:  ESP32-S3-WROOM-1, native USB-Serial-JTAG console.
// DAC:   2x MAX98357A. No I2C, MCLK-less. dac=0, software volume (volume.c).
// AMP:   built into the MAX98357As; shared SD/enable on AMP_EN=GPIO17 (HIGH = on).
// LED:   one WS2812 on GPIO9. IR-RX on GPIO7 (not wired yet).
//
// No rotary encoder (IR remote is the intended local control); the encoder pins
// below are free broken-out pads so config validation passes — provision real pins
// over USB if you solder an encoder on.
#pragma once
#define BOARD_NAME      "ondaire-loud-s3"

#define DEF_I2S_BCLK    14   // -> MAX98357A BCLK
#define DEF_I2S_LRCK    15   // -> MAX98357A LRCLK
#define DEF_I2S_DOUT    16   // -> MAX98357A DIN
#define DEF_I2S_MCLK    (-1) // MCLK-less (MAX98357A needs no master clock)

#define DEF_ENC_A       1    // no encoder fitted; free broken-out pads (placeholder)
#define DEF_ENC_B       2
#define DEF_ENC_SW      42

#define DEF_LED         9    // onboard WS2812 RGB
#define DEF_DAC         0    // MAX98357A — software gain, no I2C
#define DEF_AMP_EN      17   // shared MAX98357A SD/enable (HIGH = un-mute, idle-gated)

#define DEF_I2C_SDA     (-1) // no I2C-controlled DAC
#define DEF_I2C_SCL     (-1)
#define DEF_DAC_I2C_ADDR 0x00

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
