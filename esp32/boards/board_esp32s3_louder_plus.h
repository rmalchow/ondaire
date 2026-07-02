// Sonocotta Louder-ESP32-Plus (S3) — the "audio dock" carrier with a TI TAS5825M
// DSP class-D amp (2x30 W). Same firmware path as the Louder-ESP32-S3 (dac=2 →
// tas58xx.c; the register init is identical for the TAS5805M and TAS5825M) — only
// the I2C address differs (0x4C here vs 0x2D). MCLK-less; software volume in
// volume.c. Pin map from Sonocotta's ESPHome config (github.com/sonocotta/
// esp32-audio-dock, firmware/esphome/7-louder-esp32-plus/louder-esp32-s3-plus-idf.yaml).
// The Louder-ESP32-Pro uses the same TAS5825M + pin map.
//
// Chip:  ESP32-S3-WROOM-1, 8 MB flash / 8 MB octal PSRAM, native USB-Serial-JTAG.
// AMP:   TAS5825M (I2C @ 0x4C on SDA=8/SCL=9). dac=2.
// EN:    device PDN/enable on GPIO17 (dac_en) — TI power-up timing then held HIGH.
// LED:   one WS2812 on GPIO21 (GPIO9 is taken by I2C-SCL). IR-RX on GPIO7.
//
// No rotary encoder (IR remote is the intended local control); the encoder pins
// below are free broken-out pads so config validation passes — provision real pins
// over USB if you solder an encoder on.
#pragma once
#define BOARD_NAME      "ensemble-louder-plus-s3"

#define DEF_I2S_BCLK    14   // -> TAS5825M SCLK/BCK
#define DEF_I2S_LRCK    15   // -> TAS5825M LRCK
#define DEF_I2S_DOUT    16   // -> TAS5825M SDIN
#define DEF_I2S_MCLK    (-1) // MCLK-less (TAS5825M internal PLL derives clock from BCK)

#define DEF_ENC_A       1    // no encoder fitted; free broken-out pads (placeholder)
#define DEF_ENC_B       2
#define DEF_ENC_SW      10

#define DEF_LED         21   // onboard WS2812 RGB
#define DEF_DAC         2    // TAS5825M — same tas58xx.c init as the TAS5805M
#define DEF_AMP_EN      (-1) // integrated amp; enable is dac_en, no separate idle gate
#define DEF_DAC_EN      17   // TAS5825M PDN/enable (power-up timing, then held HIGH)

#define DEF_I2C_SDA     8    // TAS5825M control I2C
#define DEF_I2C_SCL     9
#define DEF_DAC_I2C_ADDR 0x4C  // ADR straps on this board (TAS5825M)

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
