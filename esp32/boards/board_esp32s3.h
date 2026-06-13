// ESP32-S3-WROOM-1 / DevKitC-1 defaults.
// Pins avoid: USB-Serial-JTAG (19/20), strapping (0/3/45/46), SPI flash (26-32).
#pragma once
#define BOARD_NAME      "ensemble-s3"

#define DEF_I2S_BCLK    5    // -> PCM5102A BCK
#define DEF_I2S_LRCK    6    // -> PCM5102A LCK
#define DEF_I2S_DOUT    7    // -> PCM5102A DIN
#define DEF_I2S_MCLK    (-1) // PCM5102A runs MCLK-less (tie its SCK to GND)

#define DEF_ENC_A       15   // rotary CLK
#define DEF_ENC_B       16   // rotary DT
#define DEF_ENC_SW      17   // rotary SW (push = mute)

#define DEF_LED         48   // onboard RGB on most S3 devkits
#define DEF_DAC         0    // PCM5102A, software gain

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
