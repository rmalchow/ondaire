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
//   DEF_DAC           0 = PCM5102A/PCM5100A/MAX98357 (software gain, no I2C),
//                     1 = PCM5122 (I2C), 2 = TAS5805M/TAS5825M (I2C), 3 = MA12070P (I2C)
//   DEF_AMP_EN        gpio that un-mutes a SEPARATE onboard class-D amp (e.g. a TPA
//                     next to a PCM510x/PCM5122 DAC): driven HIGH while audio plays
//                     and idle-gated by player.c, or -1 if none
//   DEF_I2C_SDA/SCL   control-I2C pins for an I2C DAC/amp (dac>=1), or -1/-1
//   DEF_DAC_I2C_ADDR  that DAC's 7-bit I2C address (0x00 if no I2C DAC)
//   BOARD_HAS_APLL    1 if the I2S clock can use the APLL (enables the rate
//                     servo + the mDNS queue=1 capability); else 0 (accept drift)
//
// Optional (default -1 if a board header does not define them):
//   DEF_DAC_EN        DAC/amp hard-enable pin the DAC driver owns — driven with the
//                     part's power-up timing then held asserted (PCM5122 enable,
//                     TAS58xx PDN, MA12070P active-low EN). Not idle-gated.
//   DEF_MA_MUTE       MA12070P mute line (LOW=muted, HIGH=unmuted); MA12070P only.
#pragma once
#include "sdkconfig.h"

#if defined(CONFIG_ONDAIRE_BOARD_ESP32S3)
#include "board_esp32s3.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_SUPERMINI)
#include "board_esp32s3_supermini.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_ZERO)
#include "board_esp32s3_zero.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_AMPED_PLUS)
#include "board_esp32s3_amped_plus.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_HIFI)
#include "board_esp32s3_hifi.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_AMPED)
#include "board_esp32s3_amped.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_LOUD)
#include "board_esp32s3_loud.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_HIFI_PLUS)
#include "board_esp32s3_hifi_plus.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_LOUDER)
#include "board_esp32s3_louder.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_LOUDER_PLUS)
#include "board_esp32s3_louder_plus.h"
#elif defined(CONFIG_ONDAIRE_BOARD_ESP32S3_LOUD_PLUS)
#include "board_esp32s3_loud_plus.h"
#else
#include "board_generic.h"
#endif

// Optional board capabilities default to "absent" so headers only declare what they
// have (see the required/optional list above).
#ifndef DEF_DAC_EN
#define DEF_DAC_EN  (-1)
#endif
#ifndef DEF_MA_MUTE
#define DEF_MA_MUTE (-1)
#endif
