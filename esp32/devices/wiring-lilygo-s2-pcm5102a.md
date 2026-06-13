# Wiring — LilyGo T8 ESP32-S2 → PCM5102A I2S DAC

Default wiring for the ensemble playback node on the **LilyGo T8 ESP32-S2**
driving the purple **GY-PCM5102** (PCM5102A) DAC. These are the firmware's
power-on defaults (`boards/board_esp32s2.h`); every pin is re-provisionable over
USB, so if you wire it differently just set the matching pins in the flasher's
device-settings panel.

See also: [`lilygo-t8-esp32-s2.md`](lilygo-t8-esp32-s2.md),
[`pcm5102a-dac.md`](pcm5102a-dac.md), and the visual
[`wiring-lilygo-s2-pcm5102a.svg`](wiring-lilygo-s2-pcm5102a.svg).

## Connections

| PCM5102A pin | Connect to (LilyGo S2) | Signal | Notes |
|--------------|------------------------|--------|-------|
| **VIN** | `3V3` | power | board regulates to 3.3 V; 5 V also works on most GY-PCM5102 modules |
| **GND** | `GND` | ground | common ground (share with the encoder) |
| **BCK** | `GPIO36` | I2S bit clock (BCLK) | `i2s_bclk` |
| **LCK** | `GPIO35` | I2S word select (LRCLK/WS) | `i2s_lrck` |
| **DIN** | `GPIO37` | I2S data (DOUT from MCU) | `i2s_dout` |
| **SCK** | `GND` | system/master clock | **tie to GND** — the module's onboard PLL makes its own clock; the firmware runs MCLK-less (`i2s_mclk = -1`) |
| **L / R / G** | (onboard 3.5 mm jack) | line/headphone out | left / right / ground — already routed to the jack |

### PCM5102A config jumpers (solder pads H1L–H4L, leave at default)
- **FMT → GND** = I2S format (default).
- **XSMT → 3V3** = un-mute / soft-unmute enabled (default; if pulled low the DAC stays muted).
- **FLT, DEMP → GND** = normal filter, de-emphasis off (default).

## ASCII schematic

```
   LilyGo T8 ESP32-S2                         GY-PCM5102 (PCM5102A)
  ┌───────────────────┐                      ┌──────────────────────┐
  │               3V3 ●──────────────────────● VIN                   │
  │               GND ●───────────┬──────────● GND                   │
  │                               └──────────● SCK   (tie low → PLL) │
  │            GPIO36 ●──────────────────────● BCK                   │
  │            GPIO35 ●──────────────────────● LCK                   │
  │            GPIO37 ●──────────────────────● DIN                   │
  │                   │                      │      L ○─┐            │
  │                   │                      │      R ○─┼─► 3.5mm    │
  │                   │                      │      G ○─┘    jack    │
  └───────────────────┘                      └──────────────────────┘

  Rotary encoder (KY-040/HW-040), for reference — see wiring table in README:
     CLK→GPIO4   DT→GPIO5   SW→GPIO6   +→3V3   GND→GND
```

## Sanity checklist
1. `SCK` on the DAC **must** go to `GND` (MCLK-less). A floating SCK is the most
   common cause of silence/noise on these boards.
2. Share a common `GND` between the LilyGo, the DAC, and the encoder.
3. After flashing, use the web flasher's **Test tone** button to confirm wiring
   before joining a group — a clean 1 kHz tone means BCK/LCK/DIN are correct.
4. The S2's USB D-/D+ are `GPIO19/20` and the SPI flash uses `GPIO26–32` — do not
   reuse those for I2S or the encoder.
