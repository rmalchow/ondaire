// Sonocotta Loud-ESP32-Plus (S3) — the "audio dock" carrier with an Infineon
// MA12070P DSP class-D amp (2x60 W into 4Ω). The MA12070P is an I2C-controlled amp
// that needs a small register init (dac=3 → ma12070p.c: I2S format + error-handler
// reset + 0 dB), plus an active-LOW enable and a separate mute line. Software volume
// (volume.c) does the attenuation. Pin map from Sonocotta's ESPHome config
// (github.com/sonocotta/esp32-audio-dock, firmware/esphome/6-loud-esp32-plus/
// loud-esp32-s3-plus-idf.yaml).
//
// Chip:  ESP32-S3-WROOM-1, 8 MB flash / 8 MB octal PSRAM, native USB-Serial-JTAG.
// AMP:   MA12070P (I2C @ 0x20 on SDA=8/SCL=9). dac=3.
// EN:    active-LOW enable on GPIO17 (dac_en) — ma12070p.c drives it LOW to run.
// MUTE:  GPIO18 (DEF_MA_MUTE) — LOW=muted while configuring, HIGH=unmuted.
// LED:   one WS2812 on GPIO21 (GPIO9 is taken by I2C-SCL). IR-RX on GPIO7.
//
// No rotary encoder (IR remote is the intended local control); the encoder pins
// below are free broken-out pads so config validation passes — provision real pins
// over USB if you solder an encoder on.
#pragma once
#define BOARD_NAME      "ensemble-loud-plus-s3"

#define DEF_I2S_BCLK    14   // -> MA12070P BCK
#define DEF_I2S_LRCK    15   // -> MA12070P LRCK/WS
#define DEF_I2S_DOUT    16   // -> MA12070P SDATA
#define DEF_I2S_MCLK    (-1) // MCLK-less (runs off the I2S clocks)

#define DEF_ENC_A       1    // no encoder fitted; free broken-out pads (placeholder)
#define DEF_ENC_B       2
#define DEF_ENC_SW      10

#define DEF_LED         21   // onboard WS2812 RGB
#define DEF_DAC         3    // MA12070P — needs the I2C init (ma12070p.c) to make sound
#define DEF_AMP_EN      (-1) // integrated amp; enable/mute handled by ma12070p.c
#define DEF_DAC_EN      17   // MA12070P enable — ACTIVE-LOW (ma12070p.c drives it LOW)
#define DEF_MA_MUTE     18   // MA12070P mute line (LOW=muted, HIGH=unmuted)

#define DEF_I2C_SDA     8    // MA12070P control I2C
#define DEF_I2C_SCL     9
#define DEF_DAC_I2C_ADDR 0x20  // AD0=AD1=0 straps on this board

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
