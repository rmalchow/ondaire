// Sonocotta HiFi-ESP32-S3 — the "audio dock" carrier with a TI PCM5100A I2S DAC,
// line-out only (no onboard amp). The PCM5100A is hardware-strapped exactly like
// the PCM5102A boards: register-less (no I2C) and MCLK-less (internal PLL derives
// the system clock from BCK), so the plain dac=0 path drives it with no init and
// software volume (volume.c). Pin map from Sonocotta's ESPHome config
// (github.com/sonocotta/esp32-audio-dock, firmware/esphome/1-hifi-esp32/
// hifi-esp32-s3-idf.yaml).
//
// Chip:  ESP32-S3-WROOM-1, native USB-Serial-JTAG (the console the web flasher's
//        Improv + JSON config talk over — no external UART bridge).
// DAC:   PCM5100A, line-out. No I2C, MCLK-less. dac=0.
// LED:   one WS2812 (addressable RGB) on GPIO9. IR-RX on GPIO7 (not wired yet).
//
// No rotary encoder on this board (IR remote is the intended local control, and IR
// isn't wired in firmware yet). The encoder pins below are just valid, free,
// broken-out pads so config validation passes — provision real pins over USB if you
// solder an encoder on.
#pragma once
#define BOARD_NAME      "ensemble-hifi-s3"

#define DEF_I2S_BCLK    14   // -> PCM5100A BCK
#define DEF_I2S_LRCK    15   // -> PCM5100A LRCK
#define DEF_I2S_DOUT    16   // -> PCM5100A DIN
#define DEF_I2S_MCLK    (-1) // MCLK-less (PCM5100A internal PLL derives SCK from BCK)

#define DEF_ENC_A       1    // no encoder fitted; free broken-out pads (placeholder)
#define DEF_ENC_B       2
#define DEF_ENC_SW      42

#define DEF_LED         9    // onboard WS2812 RGB
#define DEF_DAC         0    // PCM5100A — software gain, no I2C
#define DEF_AMP_EN      (-1) // line-out only, no onboard amp

#define DEF_I2C_SDA     (-1) // no I2C-controlled DAC
#define DEF_I2C_SCL     (-1)
#define DEF_DAC_I2C_ADDR 0x00

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
