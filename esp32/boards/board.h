// board.h — selects the per-board default pin map + capabilities from the
// Kconfig board choice. Every DEF_* value is only a power-on default: the
// provisioner overrides them in NVS (config.c), so one image fits many wirings.
//
// Each board header must define:
//   BOARD_NAME        string, for logs / mDNS default name prefix
//   DEF_I2S_BCLK/LRCK/DOUT   I2S pins to the DAC (BCK / LCK / DIN)
//   DEF_I2S_MCLK      master clock pin, or -1 if the DAC runs MCLK-less
//   DEF_ENC_A/ENC_B/ENC_SW   rotary encoder CLK / DT / SW pins
//   DEF_LED           status LED gpio, or -1 if none
//   DEF_DAC           0 = PCM5102A (software gain), 1 = PCM5122 (I2C volume)
//   BOARD_HAS_APLL    1 if the I2S clock can use the APLL (enables the rate
//                     servo + the mDNS queue=1 capability); else 0 (accept drift)
#pragma once
#include "sdkconfig.h"

#if defined(CONFIG_ENSEMBLE_BOARD_ESP32S3)
#include "board_esp32s3.h"
#elif defined(CONFIG_ENSEMBLE_BOARD_ESP32S3_SUPERMINI)
#include "board_esp32s3_supermini.h"
#elif defined(CONFIG_ENSEMBLE_BOARD_ESP32S3_ZERO)
#include "board_esp32s3_zero.h"
#else
#include "board_generic.h"
#endif
