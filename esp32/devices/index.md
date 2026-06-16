# Device Pinout Reference Pack

Pinout references for the hardware used in this project. Each device has a
markdown sheet with a full pin table, notes, and cited sources; most include a
downloaded reference image.

The browser flasher currently targets the **ESP32-S3 Super Mini** only, so its
sheet and the parts it pairs with live here at the top level. Other boards that
once worked but aren't on the current path are kept under [`later/`](later/) for
reference.

| Device | Reference | Pinout image |
|--------|-----------|--------------|
| ESP32-S3 Super Mini (22.5×18 mm, USB-C, WS2812 on GPIO48) | [esp32-s3-super-mini.md](esp32-s3-super-mini.md) | _(in-sheet pin tables)_ |
| PCM5102A I2S DAC — GY-PCM5102 (purple board) | [pcm5102a-dac.md](pcm5102a-dac.md) | [pcm5102a-dac.png](pcm5102a-dac.png) |
| KY-040 / HW-040 rotary encoder | [ky-040-rotary-encoder.md](ky-040-rotary-encoder.md) | [ky-040-rotary-encoder.jpg](ky-040-rotary-encoder.jpg) |

## Wiring diagram

| Build | Sheet | Diagram |
|-------|-------|---------|
| ESP32-S3 Super Mini → PCM5102A DAC (+ encoder) | [esp32-s3-super-mini.md](esp32-s3-super-mini.md) | [esp32-s3-super-mini-wiring-pcm5102a.svg](esp32-s3-super-mini-wiring-pcm5102a.svg) |

## Other boards (`later/`)

Not on the current flasher path, kept for reference:

| Device | Reference | Pinout image |
|--------|-----------|--------------|
| ESP32-S3-WROOM-1 / ESP32-S3-DevKitC-1 (USB-C, BOOT+RST) | [later/esp32-s3-wroom-1.md](later/esp32-s3-wroom-1.md) | [later/esp32-s3-wroom-1.jpg](later/esp32-s3-wroom-1.jpg) |
| LilyGo T8-S2 (ESP32-S2-WROOM, dual-DIP) | [later/lilygo-t8-esp32-s2.md](later/lilygo-t8-esp32-s2.md) | [later/esp32-s2-wroom-module.jpg](later/esp32-s2-wroom-module.jpg) |
| LilyGo T8 ESP32-S2 → PCM5102A DAC (+ encoder) | [later/wiring-lilygo-s2-pcm5102a.md](later/wiring-lilygo-s2-pcm5102a.md) | [later/wiring-lilygo-s2-pcm5102a.svg](later/wiring-lilygo-s2-pcm5102a.svg) |

## Quick cross-reference

- **ESP32 USB D-/D+** are **GPIO19 / GPIO20** on both the S3 and the S2.
- **Strapping pins** — S3: GPIO0/3/45/46; S2: GPIO0/45/46. Don't drive at reset.
- **Internal SPI-flash bus** (GPIO26–32, +33–37 on octal-PSRAM S3) is not on
  the headers and must not be reused.
- **PCM5102A I2S** — tie SCK to GND (onboard PLL), XSMT high to un-mute, FMT low
  for I2S; line-level L/R/G out.
- **KY-040** — CLK/DT quadrature with 10k pull-ups; SW active-low button.
