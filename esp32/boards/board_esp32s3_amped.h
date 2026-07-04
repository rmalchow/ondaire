// Sonocotta Amped-ESP32-S3 — PCM5100A DAC + a TPA3110/TPA3128 class-D speaker amp
// on the "audio dock" carrier (2x25 W). Like the HiFi-ESP32-S3 plus an amp gated by
// AMP_EN: the PCM5100A is hardware-strapped (no I2C, MCLK-less), so dac=0 drives it
// and only the amp needs a GPIO. (The -Plus variant swaps the DAC for a PCM5122
// that needs an I2C init — that is the separate esp32s3-amped-plus profile.)
// Pin map from github.com/sonocotta/esp32-audio-dock, firmware/esphome/
// 4-amped-esp32/amped-esp32-s3-idf.yaml.
//
// Chip:  ESP32-S3-WROOM-1, native USB-Serial-JTAG console.
// DAC:   PCM5100A. No I2C, MCLK-less. dac=0, software volume (volume.c).
// AMP:   TPA3110/3128, un-muted by AMP_EN=GPIO17 (driven HIGH only while audio is
//        actually playing; player.c drops it after idle to avoid amp draw/pops).
// LED:   one WS2812 on GPIO9. IR-RX on GPIO7 (not wired yet).
//
// No rotary encoder (IR remote is the intended local control); the encoder pins
// below are free broken-out pads so config validation passes — provision real pins
// over USB if you solder an encoder on.
#pragma once
#define BOARD_NAME      "ondaire-amped-s3"

#define DEF_I2S_BCLK    14   // -> PCM5100A BCK
#define DEF_I2S_LRCK    15   // -> PCM5100A LRCK
#define DEF_I2S_DOUT    16   // -> PCM5100A DIN
#define DEF_I2S_MCLK    (-1) // MCLK-less (PCM5100A internal PLL derives SCK from BCK)

#define DEF_ENC_A       1    // no encoder fitted; free broken-out pads (placeholder)
#define DEF_ENC_B       2
#define DEF_ENC_SW      42

#define DEF_LED         9    // onboard WS2812 RGB
#define DEF_DAC         0    // PCM5100A — software gain, no I2C
#define DEF_AMP_EN      17   // TPA amp un-mute (HIGH while playing, idle-gated)

#define DEF_I2C_SDA     (-1) // no I2C-controlled DAC
#define DEF_I2C_SCL     (-1)
#define DEF_DAC_I2C_ADDR 0x00

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
