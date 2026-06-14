# ensemble playback-node firmware (ESP32)

A receive-only **ensemble playback node** on an ESP32 + an I2S DAC: it is
discovered over mDNS, driven by a master over the v2 control plane, and plays a
group's audio in lock-step like any other room. It implements the full wire
protocol in [`../docs/developer/player-protocol.md`](../docs/developer/player-protocol.md) (the Go reference
is [`../cmd/player`](../cmd/player)); the firmware + hardware design is
[`../docs/developer/esp32.md`](../docs/developer/esp32.md).

**Scope (v1):** full playback protocol + mDNS advertise + USB provisioning. No
on-device HTTP, no master role, no SPA — Wi-Fi and all pin/DAC settings are
written over USB by the [web flasher](https://github.com/) (`site/flash.html`)
or any serial client.

## Hardware

See [`devices/`](devices/) for pinout sheets and wiring references.

- **Board:** a **PSRAM-equipped ESP32** — ESP32-S3-WROOM-1 (DevKitC-1) or a classic
  ESP32-WROVER. Non-PSRAM parts are out of scope (see `docs/developer/esp32.md`); other PSRAM
  chips are a board profile away — see *Adding a board* below.
- **DAC:** PCM5102A over I2S (the purple GY-PCM5102 module; software volume).
- **Encoder:** KY-040 / EC11 for local volume + mute (optional).

Default pins (`boards/board_<chip>.h`, all re-provisionable over USB):

| Function | S3 |
|----------|---:|
| I2S BCK / LCK / DIN | 5 / 6 / 7 |
| I2S MCLK | none (tie DAC SCK→GND) |
| Encoder CLK / DT / SW | 15 / 16 / 17 |

## Build

ESP-IDF **5.4+** (developed against 6.1). With a local IDF:

```sh
./build.sh esp32s3            # → build-esp32s3/ensemble-fw-esp32s3.bin (merged image)
./build.sh esp32s3 flash      # build + flash an attached board
./build.sh esp32s3 monitor    # build + flash + serial monitor
```

`build.sh` sources `~/esp/esp-idf/export.sh` (or `$IDF_PATH`) if present, else
falls back to the `espressif/idf` Docker image — so it runs with no toolchain
installed. CI builds the same way (`.gitlab-ci.yml` `firmware` job) and publishes
the merged images to the [flasher page](../site) and tagged releases.

Or drive `idf.py` directly:

```sh
. ~/esp/esp-idf/export.sh
idf.py -B build-esp32s3 -DBOARD=esp32s3 set-target esp32s3
idf.py -B build-esp32s3 -DBOARD=esp32s3 build
```

> The Opus decoder comes from the `esphome/micro-opus` managed component, which
> patches the Opus source at configure time — your build host needs the `patch`
> utility on PATH (the `espressif/idf` image and most Linux distros have it;
> `sudo dnf install patch` / `sudo apt install patch` if not).

## Flash & provision (no toolchain)

Open **`flash.html`** on the marketing site in Chrome/Edge:

1. Plug the board in via USB-C → **Install** (ESP Web Tools picks the right
   build for your chip). First flash may need download mode: hold **BOOT**, tap
   **RESET**, release BOOT.
2. **Connect** → set Wi-Fi (2.4 GHz) + confirm the I2S/encoder pins.
3. **Test tone** to verify DAC wiring → **Save** → **Reboot**.

The node joins the LAN, the cluster discovers it (`role=playback` over mDNS),
and it becomes assignable to any group. Turn the knob for volume.

### Config-over-serial protocol

Line-delimited JSON on the USB-CDC console (`115200`, baud ignored on native
USB). Scriptable without the web page:

```
→  {"cmd":"get"}
←  {"ok":true,"cfg":{"id":"…","i2s_bclk":36,"i2s_lrck":35,"i2s_dout":37,…}}
→  {"cmd":"set","cfg":{"wifi_ssid":"home","wifi_pass":"…","i2s_dout":37}}
←  {"ok":true}                       # validated + written to NVS (bad config rejected)
→  {"cmd":"test","what":"tone"}      # 1 kHz on the configured I2S
→  {"cmd":"reboot"}
```

## Adding a board

1. Add `boards/board_<chip>.h` with the default pins + `BOARD_HAS_APLL`.
2. Add a `CONFIG_ENSEMBLE_BOARD_<CHIP>` choice in `main/Kconfig.projbuild` and a
   `board.h` include.
3. Add `sdkconfig.defaults.<chip>` (target, flash size, console).
4. Add the board to the CI `firmware` matrix and the flasher `firmware.builds`
   in `../site/content.mjs`.

## Layout

`main/` — `wire` (framing), `config` (NVS), `clock` (follower), `net_audio`
(data plane + clock + keepalive), `player` (jitter buffer + playout), `opus_dec`,
`i2s_out`, `servo`, `control` (CONTROL_PORT v2 commands + STATUS), `status`,
`mdns_adv`, `encoder`, `volume`, `netif` (Wi-Fi), `console` (USB JSON), `app_main`.

## Conformance

The bar is the reference client + the e2e leg (`../scripts/e2e.sh`, `docs/developer/esp32.md`
§8): subscribe to a live group, play in sync (no dropouts, sample-aligned within
the buffer), survive `RECONFIG`/master change, and never join cluster membership
or codec negotiation. Bench check: run a software member and the ESP node in one
group on one track — both speakers phase-aligned (use the auto-calibration to
remove the fixed per-device offset).
