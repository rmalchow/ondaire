# ESP32-S3-WROOM-1 / ESP32-S3-DevKitC-1

USB-C devkit with two 22-pin headers (J1 and J3), BOOT + RST (EN) buttons, an
RGB LED on GPIO48, and **two USB-C ports** (one for the onboard UART bridge,
one wired to the native USB-OTG of the chip).

![ESP32-S3-DevKitC-1 pinout](esp32-s3-wroom-1.jpg)

## Chip / module

- SoC: ESP32-S3 (dual-core Xtensa LX7 @ 240 MHz, Wi-Fi b/g/n + BLE 5)
- Module: ESP32-S3-WROOM-1 (no PSRAM) or -1U variants; some carry 2/8 MB PSRAM
- 45 physical GPIOs, native USB-OTG, 4x SPI, 2x I2C, 3x UART, 2x I2S, RMT,
  LED PWM, 2x 12-bit ADC, 14x capacitive touch, LCD/cam (DVP) interface

## Header pinout

The board breaks out every GPIO except the internal SPI-flash/PSRAM bus.
Below is the logical map (left header J1 + right header J3). Power/ground pins
are marked; everything else is a usable GPIO with the listed alternate
functions.

### Left side (J1)

| Pin label | GPIO | Notable alternate functions |
|-----------|------|------------------------------|
| 3V3       | —    | 3.3 V output |
| 3V3       | —    | 3.3 V output |
| RST/EN    | —    | Chip reset (active low) |
| GPIO4     | 4    | ADC1_CH3, TOUCH4, RTC |
| GPIO5     | 5    | ADC1_CH4, TOUCH5, RTC |
| GPIO6     | 6    | ADC1_CH5, TOUCH6, RTC |
| GPIO7     | 7    | ADC1_CH6, TOUCH7, RTC |
| GPIO15    | 15   | ADC2_CH4, U0RTS, XTAL_32K_P, RTC |
| GPIO16    | 16   | ADC2_CH5, U0CTS, XTAL_32K_N, RTC |
| GPIO17    | 17   | ADC2_CH6, U1TXD, RTC |
| GPIO18    | 18   | ADC2_CH7, U1RXD, CLK_OUT3, RTC |
| GPIO8     | 8    | ADC1_CH7, TOUCH8, SUBSPICS1, RTC |
| GPIO3     | 3    | ADC1_CH2, TOUCH3, RTC, **strapping** (JTAG src select) |
| GPIO46    | 46   | **strapping** (boot mode), input-only-ish at boot, LOG |
| GPIO9     | 9    | ADC1_CH8, TOUCH9, FSPIHD, SUBSPIHD, RTC |
| GPIO10    | 10   | ADC1_CH9, TOUCH10, FSPICS0, FSPIIO4, RTC |
| GPIO11    | 11   | ADC2_CH0, TOUCH11, FSPID, FSPIIO5, RTC |
| GPIO12    | 12   | ADC2_CH1, TOUCH12, FSPICLK, FSPIIO6, RTC |
| GPIO13    | 13   | ADC2_CH2, TOUCH13, FSPIQ, FSPIIO7, RTC |
| GPIO14    | 14   | ADC2_CH3, TOUCH14, FSPIWP, FSPIDQS, RTC |
| 5V        | —    | 5 V (VBUS / VIN) |
| GND       | —    | Ground |

### Right side (J3)

| Pin label | GPIO | Notable alternate functions |
|-----------|------|------------------------------|
| GND       | —    | Ground |
| GPIO43    | 43   | U0TXD, CLK_OUT1 (default UART0 TX, used by serial console) |
| GPIO44    | 44   | U0RXD, CLK_OUT2 (default UART0 RX, used by serial console) |
| GPIO1     | 1    | ADC1_CH0, TOUCH1, RTC |
| GPIO2     | 2    | ADC1_CH1, TOUCH2, RTC |
| GPIO42    | 42   | MTMS (JTAG) |
| GPIO41    | 41   | MTDI (JTAG), CLK_OUT1 |
| GPIO40    | 40   | MTDO (JTAG), CLK_OUT2 |
| GPIO39    | 39   | MTCK (JTAG), CLK_OUT3, SUBSPICS1 |
| GPIO38    | 38   | FSPIWP, SUBSPIWP, **RGB_LED** on -1 N8 boards (also GPIO48 on others) |
| GPIO37    | 37   | SPIDQS, FSPIQ, SUBSPIQ (octal-PSRAM bus on N8R8 — avoid) |
| GPIO36    | 36   | SPIIO7, FSPICLK, SUBSPICLK (octal-PSRAM bus on N8R8 — avoid) |
| GPIO35    | 35   | SPIIO6, FSPID, SUBSPID (octal-PSRAM bus on N8R8 — avoid) |
| GPIO0     | 0    | **strapping** (BOOT button), RTC |
| GPIO45    | 45   | **strapping** (VDD_SPI voltage select) |
| GPIO48    | 48   | SPICLK_N, SUBSPICLK_N, **onboard RGB LED (WS2812)** |
| GPIO47    | 47   | SPICLK_P, SUBSPICLK_P |
| GPIO21    | 21   | RTC |
| GPIO20    | 20   | **USB D+**, ADC2_CH9, U1CTS, CLK_OUT1, RTC |
| GPIO19    | 19   | **USB D-**, ADC2_CH8, U1RTS, CLK_OUT2, RTC |
| 5V        | —    | 5 V (VBUS / VIN) |
| GND       | —    | Ground |

## Strapping pins — handle with care

These are sampled at reset and decide boot mode / flash voltage. Avoid driving
them at power-up, and never hard-tie them without knowing the required level:

- **GPIO0** — BOOT. Low at reset = download (bootloader) mode; high/floating =
  normal boot. Wired to the BOOT button.
- **GPIO3** — JTAG signal source / boot config; leave floating unless needed.
- **GPIO45** — VDD_SPI (flash/PSRAM voltage) select. Must match module.
- **GPIO46** — boot-message / ROM-log enable, also a boot-mode strap.

## SPI-flash / PSRAM pins — do NOT reuse

The module's internal flash (and PSRAM on R8 parts) uses **GPIO26–GPIO32**
(SPICS1, SPIHD, SPIWP, SPICS0, SPICLK, SPIQ, SPID). These are not on the
headers and must never be repurposed. On **octal-PSRAM (N8R8)** modules the
extra bus also consumes **GPIO33–GPIO37**, so avoid 35/36/37 there too.

## USB

- **GPIO19 = USB D-**, **GPIO20 = USB D+** (native USB-OTG / USB-Serial-JTAG).
  These go to the "USB" labelled Type-C connector.
- The second "UART" Type-C connector is the CP210x/CH343 bridge to UART0
  (GPIO43 TX / GPIO44 RX) for flashing/console.

## Notes

- Held BOOT + tap RST, then release BOOT to force download mode if auto-reset
  flashing fails.
- ADC2 channels are unusable while Wi-Fi is active; prefer ADC1 (GPIO1–10).
- GPIO43/44 are the default console UART — sharing them with peripherals will
  spew boot log onto your bus.

## Sources

- https://randomnerdtutorials.com/esp32-s3-devkitc-pinout-guide/
  (pinout image: https://i0.wp.com/randomnerdtutorials.com/wp-content/uploads/2024/09/ESP32-S3-pinout.jpg)
- https://docs.espressif.com/projects/esp-idf/en/latest/esp32s3/hw-reference/esp32s3/user-guide-devkitc-1.html
- https://docs.cirkitdesigner.com/component/e1c361e2-63db-4167-a9d3-600602656c8d/esp32-s3-devkitc-1
