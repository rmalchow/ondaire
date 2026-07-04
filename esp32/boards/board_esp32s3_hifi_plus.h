// Sonocotta HiFi-ESP32-Plus (S3) — the "audio dock" carrier with a TI PCM5122 DSP
// DAC, line-out only (no onboard amp). Like the Amped-ESP32-S3-Plus minus the amp:
// the PCM5122 is I2C-controlled and MCLK-less, so it stays silent until the firmware
// writes its init over I2C (dac=1 → pcm5122.c: PLL ref=BCK, un-standby, un-mute,
// 0 dB). Software volume (volume.c) does the attenuation. Pin map from Sonocotta's
// ESPHome config (github.com/sonocotta/esp32-audio-dock, firmware/esphome/
// 5-hifi-esp32-plus/hifi-esp32-s3-plus-idf.yaml).
//
// Chip:  ESP32-S3-WROOM-1, 8 MB flash / 8 MB octal PSRAM, native USB-Serial-JTAG.
// DAC:   PCM5122 (I2C @ 0x4D on SDA=42/SCL=41 — rev H1+ pin swap), line-out. dac=1.
// EN:    PCM5122 enable on GPIO4 (dac_en) — driven HIGH at boot before the I2C init.
// LED:   one WS2812 on GPIO9. IR-RX on GPIO7 (not wired yet).
//
// No rotary encoder (IR remote is the intended local control); the encoder pins
// below are free broken-out pads so config validation passes — provision real pins
// over USB if you solder an encoder on.
#pragma once
#define BOARD_NAME      "ondaire-hifi-plus-s3"

#define DEF_I2S_BCLK    14   // -> PCM5122 BCK
#define DEF_I2S_LRCK    15   // -> PCM5122 LRCK
#define DEF_I2S_DOUT    16   // -> PCM5122 DIN
#define DEF_I2S_MCLK    (-1) // MCLK-less (PCM5122 internal PLL derives SCK from BCK)

#define DEF_ENC_A       1    // no encoder fitted; free broken-out pads (placeholder)
#define DEF_ENC_B       2
#define DEF_ENC_SW      10

#define DEF_LED         9    // onboard WS2812 RGB
#define DEF_DAC         1    // PCM5122 — needs the I2C init (pcm5122.c) to make sound
#define DEF_AMP_EN      (-1) // line-out only, no onboard amp
#define DEF_DAC_EN      4    // PCM5122 enable (driven HIGH at boot, before I2C init)

#define DEF_I2C_SDA     42   // PCM5122 control I2C (rev H1+ pin map)
#define DEF_I2C_SCL     41
#define DEF_DAC_I2C_ADDR 0x4D  // ADR straps on this board (0x4C on some HW revs)

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
