// Waveshare ESP32-S3-Zero (the 23.5 x 18 mm USB-C castellated mini board).
//
// Chip:   ESP32-S3 (Xtensa dual-core LX7 @ 240 MHz), 512 KB SRAM
// Flash:  4 MB (quad SPI, internal)   PSRAM: 2 MB embedded quad (S3FH4R2, 3.3V)
// USB:    native USB-Serial-JTAG over USB-C (D- = GPIO19, D+ = GPIO20) — this
//         is the console the web flasher's Improv + JSON config talk over.
// LED:    one WS2812 (addressable RGB) on GPIO21 — the only difference from the
//         Super Mini, which puts the same LED on GPIO48.
// Wi-Fi:  2.4 GHz b/g/n + BLE 5, on-board ceramic antenna.
//
//   Castellated/through-hole pads (board silk -> GPIO):
//     one edge:   5V GND 3V3  GPIO1 2 3 4 5 6 7 8 9 10 11 12 13
//     other edge: GPIO14 ... 18, 21, 38 39 40 41 42, 43(TX) 44(RX), 45
//     under-USB pads expose the rest; GPIO19/20 are USB D-/D+ (not pads).
//
//   Avoid for app I/O:
//     GPIO19/20  native USB-Serial-JTAG (D-/D+) — leave for the console
//     GPIO0/3/45/46  strapping (boot mode, JTAG sel, VDD_SPI, ROM msg)
//     GPIO26-32  internal quad SPI flash (not broken out)
//     GPIO21     onboard WS2812 RGB LED (used as the status LED below)
//   Safe general-purpose: GPIO1 2 4 5 6 7 8 15 16 17 18 (+ ADC1 on 1-10).
//
// Pin map below matches board_esp32s3.h (DevKitC) and the Super Mini: every
// value listed is exposed and conflict-free on the S3-Zero too, so one wiring
// fits all three. Only the status LED moves (GPIO48 -> GPIO21).
#pragma once
#define BOARD_NAME      "ensemble-s3zero"

#define DEF_I2S_BCLK    5    // -> PCM5102A BCK
#define DEF_I2S_LRCK    6    // -> PCM5102A LCK
#define DEF_I2S_DOUT    7    // -> PCM5102A DIN
#define DEF_I2S_MCLK    (-1) // PCM5102A runs MCLK-less (tie its SCK to GND)

#define DEF_ENC_A       15   // rotary CLK
#define DEF_ENC_B       16   // rotary DT
#define DEF_ENC_SW      17   // rotary SW (push = mute)

#define DEF_LED         21   // onboard WS2812 RGB
#define DEF_DAC         0    // PCM5102A, software gain
#define DEF_AMP_EN      (-1) // no software amp-enable pin (PCM5102A boards)

#define DEF_I2C_SDA     (-1) // no I2C-controlled DAC (PCM5102A is pin-configured)
#define DEF_I2C_SCL     (-1)
#define DEF_DAC_I2C_ADDR 0x00

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
