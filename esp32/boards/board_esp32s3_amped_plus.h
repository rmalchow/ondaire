// Sonocotta Amped-ESP32-S3-Plus — rev C (schematic title "Amped ESP32 S3 Plus
// (PCM5122 + TPA3110)", 2026-01). A single board carrying the ESP32-S3, a
// DSP-capable I2S DAC, and a class-D speaker amp — no external DAC to wire.
//
// Chip:   ESP32-S3-WROOM-1: Xtensa dual-core LX7 @ 240 MHz, 512 KB SRAM
// Flash:  8 MB (quad SPI, internal)    PSRAM: 8 MB embedded OCTAL
//         (The rev-C schematic marks the module "N16R8" = 16 MB flash, but the
//         shipping board reports 8 MB — `esptool flash_id` / boot log — and
//         Sonocotta's own ESPHome uses 8 MB. 16 MB boot-loops. PSRAM is 8 MB.)
// USB:    native USB-Serial-JTAG over USB-C (D- = GPIO19, D+ = GPIO20) — the
//         console the web flasher's Improv + JSON config talk over. (The board's
//         "USB-to-serial bridge" block is just the USB-C connector + CC/ESD; the
//         data lines land on the S3's native USB, exactly like the other S3s.)
// DAC:    TI PCM5122 (I2C control @ 0x4D on SDA=18/SCL=8) feeding a TPA3110/3128
//         class-D amp. See the DAC / amp note below — we do NOT use I2C today.
// LED:    one WS2812 (addressable RGB) on GPIO21 (RGB_LED header).
// Wi-Fi:  2.4 GHz b/g/n + BLE 5 (WROOM-1 module, on-board PCB antenna; an
//         external-antenna module variant also ships).
//
//   Confirmed rev-C GPIO map (from the rev-C schematic + Sonocotta's ESPHome /
//   Squeezelite configs — github.com/sonocotta/esp32-audio-dock):
//     I2S:   BCLK=14  LRCK/WS=15  DOUT=16   (MCLK-less — see below)
//     I2C:   SDA=18   SCL=8       PCM5122 @ 0x4D
//     AMP:   AMP_EN=17 (drive HIGH to un-mute the TPA amp; see DEF_AMP_EN)
//     misc:  IR-RX=7, RGB LED=21, SPI(OLED)=11/12/13 + 38/47/48, W5500=5/6/10
//
//   ** rev-C difference vs older Amped revs: "No DAC_EN, only AMP_EN." ** The
//   PCM5122's soft-mute (XSMT) is hardwired asserted-off on the PCB, so the DAC
//   comes up un-muted on its own — there is no DAC-enable GPIO to drive. The amp
//   is the only mute control, on GPIO17.
//
//   MCLK: none. The PCM5122 has an internal PLL and derives its system clock from
//   BCK, so it runs MCLK-less like the PCM5102A boards (DEF_I2S_MCLK = -1).
//
//   DAC note: the PCM5122 is strapped in I2C (software) control mode and runs
//   MCLK-less, so — unlike the PCM5102A boards — it stays SILENT (line-out AND
//   amp) until told over I2C to reference BCK for its PLL. DEF_DAC=1 makes
//   player_init run pcm5122.c's minimal init (PLL ref=BCK, un-standby, un-mute,
//   0 dB) over DEF_I2C_SDA/SCL. Playback volume is still done in software
//   (volume.c) with the DAC at unity; hardware (I2C) volume is a future option.
//
// No rotary encoder on this board (IR remote is the intended local control, and
// IR isn't wired in firmware yet). The encoder pins below are just valid, free,
// broken-out pads so config validation passes — provision real pins over USB if
// you solder an encoder on.
#pragma once
#define BOARD_NAME      "ensemble-amped-s3"

#define DEF_I2S_BCLK    14   // -> PCM5122 BCK
#define DEF_I2S_LRCK    15   // -> PCM5122 LRCK
#define DEF_I2S_DOUT    16   // -> PCM5122 DIN
#define DEF_I2S_MCLK    (-1) // MCLK-less (PCM5122 internal PLL derives SCK from BCK)

#define DEF_ENC_A       1    // no encoder fitted; free broken-out pads (placeholder)
#define DEF_ENC_B       2
#define DEF_ENC_SW      9

#define DEF_LED         21   // onboard WS2812 RGB
#define DEF_DAC         1    // PCM5122 — needs the I2C init below to make sound
#define DEF_AMP_EN      17   // TPA amp un-mute: driven HIGH at boot (kept ALWAYS_ON)

// PCM5122 control I2C (software mode). The firmware writes a minimal init over
// this bus at boot (pcm5122.c) — REQUIRED for audio: MCLK-less, so the DAC's PLL
// must be pointed at BCK or it never clocks and stays silent (line-out + amp).
#define DEF_I2C_SDA     18
#define DEF_I2C_SCL     8
#define DEF_DAC_I2C_ADDR 0x4D  // ADR straps on this board

#define BOARD_HAS_APLL  0    // the S3's I2S peripheral has no APLL (uses PLL_F160M)
