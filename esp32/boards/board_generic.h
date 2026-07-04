// Generic fallback profile — conservative defaults; provision the real pins
// over USB. APLL servo left off (unknown silicon); the node advertises queue=0
// and accepts crystal drift (skip/silence floor) until told otherwise.
#pragma once
#define BOARD_NAME      "ondaire"

#define DEF_I2S_BCLK    5
#define DEF_I2S_LRCK    6
#define DEF_I2S_DOUT    7
#define DEF_I2S_MCLK    (-1)

#define DEF_ENC_A       15
#define DEF_ENC_B       16
#define DEF_ENC_SW      17

#define DEF_LED         (-1)
#define DEF_DAC         0
#define DEF_AMP_EN      (-1) // provision over USB if your board has an amp-enable pin

#define DEF_I2C_SDA     (-1) // provision if you have an I2C-controlled DAC (PCM5122)
#define DEF_I2C_SCL     (-1)
#define DEF_DAC_I2C_ADDR 0x00

#define BOARD_HAS_APLL  0
