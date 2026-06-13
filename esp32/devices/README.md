# Device Pinout Reference Pack

Pinout references for the hardware used in this project. Each device has a
markdown sheet with a full pin table, notes, and cited sources; most include a
downloaded reference image.

| Device | Reference | Pinout image |
|--------|-----------|--------------|
| ESP32-S3-WROOM-1 / ESP32-S3-DevKitC-1 (USB-C, BOOT+RST) | [esp32-s3-wroom-1.md](esp32-s3-wroom-1.md) | [esp32-s3-wroom-1.jpg](esp32-s3-wroom-1.jpg) |
| LilyGo T8-S2 (ESP32-S2-WROOM, dual-DIP) | [lilygo-t8-esp32-s2.md](lilygo-t8-esp32-s2.md) | [esp32-s2-wroom-module.jpg](esp32-s2-wroom-module.jpg) |
| PCM5102A I2S DAC — GY-PCM5102 (purple board) | [pcm5102a-dac.md](pcm5102a-dac.md) | [pcm5102a-dac.png](pcm5102a-dac.png) |
| KY-040 / HW-040 rotary encoder | [ky-040-rotary-encoder.md](ky-040-rotary-encoder.md) | [ky-040-rotary-encoder.jpg](ky-040-rotary-encoder.jpg) |

## Wiring diagrams

| Build | Sheet | Diagram |
|-------|-------|---------|
| LilyGo T8 ESP32-S2 → PCM5102A DAC (+ encoder) | [wiring-lilygo-s2-pcm5102a.md](wiring-lilygo-s2-pcm5102a.md) | [wiring-lilygo-s2-pcm5102a.svg](wiring-lilygo-s2-pcm5102a.svg) |

## Quick cross-reference

- **ESP32 USB D-/D+** are **GPIO19 / GPIO20** on both the S3 and the S2.
- **Strapping pins** — S3: GPIO0/3/45/46; S2: GPIO0/45/46. Don't drive at reset.
- **Internal SPI-flash bus** (GPIO26–32, +33–37 on octal-PSRAM S3) is not on
  the headers and must not be reused.
- **PCM5102A I2S** — tie SCK to GND (onboard PLL), XSMT high to un-mute, FMT low
  for I2S; line-level L/R/G out.
- **KY-040** — CLK/DT quadrature with 10k pull-ups; SW active-low button.
