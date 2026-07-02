// Sonocotta Louder-ESP32-S3 — the "audio dock" carrier with a TI TAS5805M DSP
// class-D amp (2x32 W into 8Ω). The TAS5805M is an I2C-controlled DAC+amp that is
// MCLK-less (internal PLL from BCK) and stays silent until the firmware writes its
// register init (dac=2 → tas58xx.c: reset, BTL, 0 dB, play/unmute). Software volume
// (volume.c) does the attenuation. Pin map from Sonocotta's ESPHome config
// (github.com/sonocotta/esp32-audio-dock, firmware/esphome/3-louder-esp32/
// louder-esp32-s3-idf.yaml). The Louder-ESP32-Mini uses the same TAS5805M + pin map.
//
// Chip:  ESP32-S3-WROOM-1, 8 MB flash / 8 MB octal PSRAM, native USB-Serial-JTAG.
// AMP:   TAS5805M (I2C @ 0x2D on SDA=8/SCL=9). dac=2.
// EN:    device PDN/enable on GPIO17 (dac_en) — TI power-up timing then held HIGH.
// LED:   one WS2812 on GPIO21 (GPIO9 is taken by I2C-SCL). IR-RX on GPIO7.
//
// No rotary encoder (IR remote is the intended local control); the encoder pins
// below are free broken-out pads so config validation passes — provision real pins
// over USB if you solder an encoder on.
#pragma once
#define BOARD_NAME      "ensemble-louder-s3"

#define DEF_I2S_BCLK    14   // -> TAS5805M SCLK/BCK
#define DEF_I2S_LRCK    15   // -> TAS5805M LRCK
#define DEF_I2S_DOUT    16   // -> TAS5805M SDIN
#define DEF_I2S_MCLK    (-1) // MCLK-less (TAS5805M internal PLL derives clock from BCK)

#define DEF_ENC_A       1    // no encoder fitted; free broken-out pads (placeholder)
#define DEF_ENC_B       2
#define DEF_ENC_SW      10

#define DEF_LED         21   // onboard WS2812 RGB
#define DEF_DAC         2    // TAS5805M — needs the I2C init (tas58xx.c) to make sound
#define DEF_AMP_EN      (-1) // integrated amp; enable is dac_en, no separate idle gate
#define DEF_DAC_EN      17   // TAS5805M PDN/enable (power-up timing, then held HIGH)

#define DEF_I2C_SDA     8    // TAS5805M control I2C
#define DEF_I2C_SCL     9
#define DEF_DAC_I2C_ADDR 0x2D  // ADR straps on this board

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
