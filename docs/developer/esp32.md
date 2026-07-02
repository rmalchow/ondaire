# ESP32 player (PSRAM ESP32-S3 / WROVER)

A hardware **player**: a tiny receive-only ensemble speaker built on a
**PSRAM-equipped ESP32** (ESP32-S3-WROOM-1 or classic ESP32-WROVER) + an I2S DAC.
It is **visible and
assignable** in the cluster (it shows up in the UI, you can drop it into any
group, set its volume and delay, and auto-calibrate it) but it is a **receive-only**
node — it never sources audio (no library, can't become a master/"room") and it
does **not** run gossip. The audio wire protocol it speaks is specified in
[`player-protocol.md`](player-protocol.md); this document is the firmware + hardware + membership +
provisioning design.

> **Status: in bring-up — not yet conformant.** The firmware lives in
> [`../esp32/`](../../esp32) (build + flash in [`esp32/README.md`](../../esp32/README.md))
> and runs on real hardware (it boots, provisions over USB, joins the cluster over
> mDNS, clock-syncs, and answers the v2 control plane — first validated on a bench
> board). But **no board passes the §8 conformance bar yet** (clean, sustained,
> phase-aligned playout) —
> see the [target matrix](#boards--build-targets), where the *Supported* column is
> deliberately all-unchecked. The browser flasher is built but **intentionally
> unlinked from the marketing site** until a board is conformant (re-link by
> restoring the `flash.html` entries in `site/content.mjs`).
>
> **Two v1 deltas from earlier drafts:** (1) the node is driven by the **full v2
> control plane** — discovered over mDNS and `ATTACH`ed by a master (an earlier
> "thin HTTP API on the MCU" sketch and the legacy `/api/cluster` self-poll are
> **not** used; the existing Go master in `internal/playback/` already drives it, no
> server change).
> (2) First-run Wi-Fi setup is a **Tasmota-style captive portal** (§6.5): an
> unprovisioned board — or one whose stored creds fail to get an IP — brings up an
> open AP and a web form to set Wi-Fi + speaker name. The **USB JSON console** stays
> the wired alternative for creds *and* all pin/DAC settings, and remains the only
> way to change pins. Improv-Serial is still replaced by that JSON panel. The rate
> servo (§3.3) ships as drift telemetry only (advertises `queue=0`); the skip/silence
> floor bounds drift within `bufferMs`.

> Companion board: the [`kicad/` amp board](../../kicad) is an ESP32-S3 +
> PCM5102A + TPA3118D2 reference. The firmware shares the I2S/DAC design across
> every board class in the matrix below.

---

## Boards & build targets

The firmware is **one image, parameterized by a board profile chosen at build
time**. A board profile is two small files:

- `esp32/boards/board_<board>.h` — the default **pin map** + capability flags
  (`BOARD_HAS_APLL`, reserved pins);
- `esp32/sdkconfig.defaults.<board>` — the **console** (USB-UART bridge vs native
  USB), flash size, and PSRAM mode.

Everything a profile sets is a *board fact* (§6.1), and every pin is still
**NVS-overridable at provisioning time** — so one image fits many layouts and the
profile is just sane defaults, not a hard wiring.

### Selecting a board to build

```sh
cd esp32
./build.sh esp32s2            # Docker wrapper — no local ESP-IDF needed
# or, with ESP-IDF on PATH:
idf.py -DBOARD=esp32s2 build
```

`build.sh` maps the board name to its chip target (e.g. `esp32-wrover` → `esp32`,
`esp32s3-zero` → `esp32s3`) and selects the matching `sdkconfig.defaults.<board>`.
After editing a board's `sdkconfig.defaults.*`, delete `build-<board>/sdkconfig`
so it regenerates.

### Adding a board

1. `esp32/boards/board_<board>.h` — copy the closest sibling; fix the pin map,
   `BOARD_HAS_APLL`, and any reserved pins.
2. `esp32/sdkconfig.defaults.<board>` — console + flash size + PSRAM mode.
3. A `BOARD → chip` row in `esp32/build.sh`.
4. A matrix row in `.gitlab-ci.yml`.
5. A row in the table below, plus a **board photo** and a **wiring diagram** under
   [`esp32/devices/`](../../esp32/devices) — the flasher (§6.2) and these docs reuse
   the same assets.

### Aspirational target matrix

**Targets are PSRAM-only.** A player needs the jitter buffer + opus decoder in
SPIRAM; without PSRAM the ~320–400 KB internal-RAM budget is a knife-fight against
the Wi-Fi/lwIP stack (opus-only, trimmed buffers, init-ordering tricks) — not worth
shipping. So non-PSRAM parts (ESP32-S2, ESP32-C3, ESP32-C6, classic WROOM) are
**out of the supported matrix**; they may still boot as a bench/bring-up build, but
they are not products.

> **Legend.** *Console* is how the board flashes/provisions over USB (§6.4).
> *queue* = the rate servo can actuate the I2S clock via APLL (§3.3).
>
> **The _Supported_ column is the one that matters: a box is checked only when
> that board passes the §8 conformance bar on real hardware. Nothing is checked
> yet.**

| Board | Chip | PSRAM | Console (USB) | queue (APLL) | Reserved pins | Supported |
|-------|------|-------|---------------|:------------:|---------------|:---------:|
| ESP32-S3-DevKitC-1-N16R8 | ESP32-S3 | 8 MB | UART **+** native JTAG | no | 35–37 (octal PSRAM) | [ ] |
| Waveshare ESP32-S3-Zero | ESP32-S3 | 2 MB | native USB-Serial-JTAG | no | — | [ ] |
| Sonocotta Amped-ESP32-S3-Plus (rev C) | ESP32-S3 | 8 MB (octal) | native USB-Serial-JTAG | no | 26–37 (flash + octal PSRAM) | [x] tested |
| Sonocotta audio-dock family — HiFi / HiFi-Plus / Amped / Loud / Loud-Plus / Louder / Louder-Plus | ESP32-S3 | 8 MB (octal) | native USB-Serial-JTAG | no | 26–37 (flash + octal PSRAM) | [ ] **untested** |
| ESP32-DevKitC (WROVER) | ESP32 (classic) | 8 MB | CP2102 UART → `ttyUSB` | **yes** | 16, 17 (PSRAM) | [ ] |
| Generic / bring-your-own-pins (PSRAM) | S3 / WROVER | required | varies | varies | — | [ ] |

Boards in hand: an **ESP32-WROVER** DevKitC (8 MB PSRAM, classic dual-core **with**
APLL — the first PSRAM target on the bench), an **ESP32-S3 Super Mini** and a
**Waveshare ESP32-S3-Zero** (both S3FH4R2, 2 MB PSRAM — the small-form picks, board
profiles `esp32s3-supermini` / `esp32s3-zero`), and a **Sonocotta
Amped-ESP32-S3-Plus** rev C (8 MB flash + 8 MB octal PSRAM, integrated PCM5122 DAC +
TPA3110 amp — profile `esp32s3-amped-plus`, drives speakers directly; see
[`esp32/devices/amped-esp32-s3-plus.md`](../../esp32/devices/amped-esp32-s3-plus.md)).
The rest of the Sonocotta audio-dock family (HiFi / HiFi-Plus / Amped / Loud /
Loud-Plus / Louder / Louder-Plus, profiles `esp32s3-{hifi,hifi-plus,amped,loud,
loud-plus,louder,louder-plus}`) ships as **untested** board profiles — pin-map
correct from the vendor schematics/ESPHome, compiled + flashable, but not yet
bench-verified; the web flasher badges them and their DAC drivers (`tas58xx.c`,
`ma12070p.c`) are new. See
[`esp32/devices/sonocotta-audio-dock.md`](../../esp32/devices/sonocotta-audio-dock.md).
Out of scope: non-PSRAM ESP32s (above); **Nordic** parts
(BLE/Thread radios, no Wi-Fi); and the **RP2040 / Pico W** family (a different SDK,
and the plain Pico W is RAM-tight at 264 KB — the **Pico 2 W**, 520 KB, would be the
only sane non-Espressif port).

---

## 1. What it must do (recap)

From [`player-protocol.md`](player-protocol.md), a conforming player:

1. **Announces over mDNS** (`role=playback` + control port + capabilities) and is
   **idle until a master ATTACHes it**. It does **not** self-discover or poll any
   HTTP API — the master finds it and drives it.
2. On **ATTACH**, subscribes to the master's `SOURCE_PORT` (UDP): `HELLO` (prime-me),
   re-`HELLO` keepalive every 5 s, `RESTART` on >2 s starvation, honor `RECONFIG`.
3. **Clock-syncs** to the master's `STREAM_PORT` (UDP) at 1 Hz: 4-timestamp NTP
   exchange, median-of-best-RTT offset, against a local monotonic clock.
4. **Plays out**: jitter-buffer received frames and emit each at
   `pts − offset + bufferMs` on the local clock; silence on gaps.
5. **Decodes opus** (the default group codec; PCM datagrams fragment on Wi-Fi —
   §1 of player-protocol.md). Opus is effectively mandatory on Wi-Fi hardware.
6. Applies the master's **SETVOL / SETDELAY / SETEQ** and answers **STATUSREQ** with
   STATUS (§6 of player-protocol.md) — so it stays controllable and visible in the UI.

Everything below is *how* a PSRAM ESP32 does that, plus local volume and a
browser-based flasher/provisioner.

---

## 1b. Membership — discovered, represented, assigned (no gossip, no HTTP on the MCU)

A player is **visible and assignable**, not a hidden fixed-IP speaker — yet it
neither gossips nor runs any HTTP. ensemble's gossip is hashicorp/memberlist (SWIM,
TCP push/pull, msgpack, LWW-doc replication) — too heavy for an MCU and unnecessary.
Instead the MCU only **advertises over mDNS** and **answers the v2 control plane**;
a master (the "room" role) discovers, represents, and drives it:

```
   ESP player                       master (a "room")
   ──────────                       ─────────────────
   mDNS advertise  ───────────────▶ discovers _ensemble._tcp (role=playback, caps)
   role=playback,id,control         │  injects a NON-GOSSIPING node record into the
                                     │  cluster doc; liveness = mDNS freshness OR STATUS
   ATTACH/SETVOL/SETDELAY  ◀─────────┤  drives it (assigned via `Following`); the UI's
   STATUS  ─────────────────────────▶  volume / delay / assignment reach it over the wire
```

What this buys, for **mDNS + one UDP control port** on the chip (no SWIM, no
msgpack, no HTTP, no doc merge):

- **Visible** in `/api/cluster` and the Rooms UI — the master injects a node record
  for the discovered player (`cluster.UpsertPlaybackNode`).
- **Assignable** — the operator drops it into a group; it reuses the `Following`
  field, and `DeriveGroups` attaches it to that master. It is **never a solo group
  of its own** and never becomes a master.
- **Controllable & calibratable** — `SETVOL`/`SETDELAY` (and auto-calibration,
  [`calibration.md`](calibration.md)) reach it over the control plane, so the UI sliders
  treat it exactly like a software player.
- **Non-mastering** — no media library, never sources audio; invisible to codec
  negotiation (the group's codec is chosen by its gossiping members only).

This needs **no on-MCU HTTP** and **no special server work**: the existing master
(`internal/playback` driver + `internal/cluster` ingest) already discovers, injects,
assigns, and drives it. Liveness for the non-gossiping player is mDNS-browse freshness
OR a recent STATUS, ageing out when both go stale.

> **Bench/bring-up fallback** (`disc_mode=1`, fixed master IP, no mDNS): the
> self-directed mode from [`player-protocol.md`](player-protocol.md) §11 — **invisible**, pinned to one
> master, not controllable through the UI. Opt-in, for segmented networks or
> bring-up only — never how a deployed player participates.

---

## 2. Hardware

### 2.1 Bill of materials (minimum)
- A **PSRAM-equipped ESP32** module (ESP32-S3-WROOM-1 with ≥2 MB PSRAM, or a
  classic ESP32-WROVER with 8 MB PSRAM), ≥4 MB flash, 2.4 GHz Wi-Fi. PSRAM is
  required — it holds the jitter buffer + opus decoder (§3.1).
- **I2S DAC** — PCM5102A (no I2C; hardware-config pins) *or* PCM5122 (I2C, has
  a hardware volume register). Line/headphone out, or feed a class-D amp
  (TPA3118 as on the companion board).
- **Rotary encoder** with push switch (EC11-style, quadrature A/B + SW) for
  volume.
- *Optional, I2C:* a small **SSD1306 OLED** (status: group, volume, sync) and/or
  a PCM5122 DAC whose volume is set over I2C.
- 5 V supply; USB-C for flashing/provisioning (native USB).

### 2.2 Default pin map (all configurable, §5)

| Function | Default GPIO | Notes |
|----------|-------------:|-------|
| I2S BCLK | 36 | bit clock |
| I2S LRCLK/WS | 35 | word select |
| I2S DOUT | 37 | data to DAC DIN |
| I2S MCLK | 0 *(opt)* | master clock; PCM5102A can run MCLK-less |
| I2C SDA | 8 *(opt)* | OLED / PCM5122 |
| I2C SCL | 9 *(opt)* | |
| Encoder A | 4 | quadrature |
| Encoder B | 5 | quadrature |
| Encoder SW | 6 | push (mute / confirm) |
| Status LED | 15 | sync/activity heartbeat |

Pins are stored in NVS and set at provisioning time, so one firmware image fits
many board layouts.

---

## 3. Firmware architecture

Single core, so cooperative FreeRTOS tasks with clear priorities. Built with
ESP-IDF (5.x). USB-CDC for console/provisioning.

```
            Wi-Fi / lwIP
                │
   ┌────────────┴───────────┐
   │   net task (prio hi)    │  one UDP socket on a local port:
   │  HELLO/keepalive/RESTART│  - sends HELLO/clock-req to SOURCE/STREAM ports
   │  recv audio+FEC+clock   │  - recvfrom → parse 24-byte header by type
   └─────┬───────────┬───────┘
         │ frames    │ clock replies
         ▼           ▼
  ┌────────────┐  ┌────────────────┐
  │ jitter buf │  │ clock follower │ esp_timer (µs) = local monotonic clock
  │  (seq map) │  │ median offset  │
  └─────┬──────┘  └───────┬────────┘
        │ opus decode      │ offset
        ▼                  ▼
  ┌──────────────────────────────────┐
  │ audio task (prio highest)        │  I2S DMA double-buffer; for each 20 ms
  │  deadline = pts−offset+bufferMs   │  slot pop frame, gain, write to I2S;
  │  APLL rate trim (the servo)       │  silence on gap; APLL ppm nudge
  └──────────────────────────────────┘
        ▲ volume                       
  ┌─────┴──────┐  ┌──────────────────┐
  │ encoder    │  │ control task     │ HTTP discovery poll (5 s), RECONFIG,
  │ (PCNT/ISR) │  │ OLED, LED, NVS   │ status, OTA check
  └────────────┘  └──────────────────┘
```

### 3.1 Memory budget (PSRAM holds the big buffers)
- Jitter buffer (compressed opus packets, or decoded PCM with room to spare) and
  the opus decoder state + scratch live in **PSRAM** (SPIRAM), so internal SRAM is
  never the constraint.
- Internal SRAM then comfortably covers Wi-Fi/lwIP ≈ **50 KB**, I2S DMA ≈ **8 KB**,
  and tasks/stacks ≈ **20 KB**.
- This is exactly why targets are PSRAM-only: on a non-PSRAM part the jitter
  buffer + decoder + Wi-Fi stack fight over ~320–400 KB and only fit opus-only with
  trimmed buffers (the abandoned S2-class budget).

### 3.2 Opus decode
Default group codec. Decode each 20 ms / 48 kHz / stereo Opus packet to PCM
before the jitter buffer. libopus decode-only on a single 240 MHz LX7 is a small
fraction of a core; it is the right choice because a 20 ms Opus packet (~320 B)
is one unfragmented datagram on Wi-Fi. (PCM mode is accepted for wired/bench use
but discouraged — see [`player-protocol.md`](player-protocol.md).)

### 3.3 Clock & the rate servo — done in hardware
The server resamples in software; the ESP32 does it for free in
the **I2S clock**. The I2S sample rate is derived from the **APLL**, whose
fractional divider can be nudged in ~ppm steps. So instead of resampling:

- Drive the **jitter-buffer fill level** (or the playout phase error) to a
  setpoint with a slow proportional controller.
- The controller output trims the APLL sample rate by ±N ppm.
- Net effect: the DAC consumes at exactly the master's effective rate; the
  buffer neither drifts empty nor full. Inaudible, no software resampling.

The server now runs DAC-pull: its blocking device write is the rate pacer and a
**phase-lock PI servo trims the resample ratio** to hold the device queue at its
phase target (`internal/sink/servo.go`). The ESP32 reaches the same end — buffer
held, drift inaudible — but **actuates the real I2S/APLL clock in hardware**
instead of resampling. Clamp to ±300 ppm; smooth heavily (Wi-Fi clock jitter).

### 3.4 Playout
`esp_timer` (64-bit µs since boot) is the local monotonic clock. The clock
follower yields `offset`; the audio task schedules frame `pts` for emission at
`MasterToLocal(pts) + bufferMs`, writing into the I2S DMA ring so audio leaves
the DAC at that instant. Missing frames (after the small reorder/FEC window) play
silence. Apply local volume (gain or DAC register) just before I2S.

---

## 4. Local volume — rotary encoder (and optional I2C)

- **Quadrature decode** via the ESP32 **PCNT** peripheral (or A/B GPIO ISR) with
  debounce; one detent = ±1 step. The push switch toggles **mute**.
- **Apply** the volume as either: (a) a digital gain on the PCM (software,
  any DAC), or (b) the DAC's hardware volume register over **I2C** (PCM5122) —
  cleaner, no bit loss. I2C is optional; without it, software gain is used.
- **Optional OLED** (I2C): show group name (from the ATTACH/STATUS state), volume %,
  sync state, and a buffer/health bar.
- **Cluster-visible volume**: the knob and the master's `SETVOL` (driven from the
  UI's per-room volume) are the *same* value — the firmware applies the knob locally
  and reports it in `STATUS`, and obeys `SETVOL` over the control plane (last writer
  wins), so the UI slider and the knob stay in sync. (In the bench fixed-master
  fallback it is local-only.)

---

## 5. Configuration (NVS)

All settings persist in NVS namespace `ensemble`. Defaults let an unprovisioned
board boot into AP/provisioning mode.

| Key | Type | Meaning |
|-----|------|---------|
| `wifi_ssid` / `wifi_pass` | str | Wi-Fi credentials (set via the flasher / JSON console) |
| `disc_mode` | u8 | `0`=mDNS auto, `1`=fixed master |
| `master_ip` / `source_port` / `stream_port` | str/u16 | fixed-master target |
| `group` | str | optional: which group/node name to follow when several exist |
| `codec` | u8 | `0`=opus (default), `1`=pcm |
| `buffer_ms` | u16 | playout buffer (default 150) |
| `i2s_bclk`/`i2s_lrck`/`i2s_dout`/`i2s_mclk` | u8 | I2S pins |
| `i2c_sda`/`i2c_scl` | i8 | control-I2C pins for an I2C DAC/amp (-1 if none) |
| `dac` | u8 | `0`=PCM510x/MAX98357 (sw gain), `1`=PCM5122, `2`=TAS5805M/5825M, `3`=MA12070P (all I2C) |
| `amp_en` | i8 | separate class-D amp un-mute pin (HIGH=on, idle-gated), -1 if none |
| `dac_en` | i8 | I2C DAC/amp hard-enable the driver owns (held on), -1 if none |
| `enc_a`/`enc_b`/`enc_sw` | u8 | rotary encoder pins |
| `vol` | u8 | last volume 0–100 (restored on boot) |
| `name` | str | friendly label (logs / OLED) |

---

## 6. Web flasher & provisioner

A **static web page** (Chrome/Edge, Web Serial API) that flashes the firmware and
writes the per-device settings over USB — no toolchain, no app install. Built by
`site/build.mjs` (`flash.html` + an [ESP Web Tools](https://esphome.github.io/esp-web-tools/)
manifest) and hosted on GitLab Pages.

> **Status:** built but **unlinked** from the site nav and download page until a
> board is conformant (§8). Re-link by restoring the `flash.html` entries in
> `site/content.mjs` (`nav` + `download.links`).

### 6.1 What the user is actually choosing — three buckets

Most of §5 is **not** a per-device decision. Sorting the config into three buckets
is what keeps the panel small and unconfusing:

| Bucket | Examples | Who sets it |
|--------|----------|-------------|
| **Board facts** | I2S/encoder pins, PSRAM mode, APLL/`queue`, console type, reserved pins | The **board profile** — chosen by *picking the board*, never typed |
| **Per-device identity / network** | `wifi_ssid`, `wifi_pass`, `name` | **The flasher** — the only bucket it must ask for |
| **Cluster / runtime** | `codec`, `buffer_ms`, `vol`, delay, EQ | The **master**, over the control plane (ATTACH/SETVOL/SETDELAY/RECONFIG) — overwritten on attach, so the flasher must **not** ask |

So the front door is just **pick board → Wi-Fi → (optional name)**: *board facts*
fill in from the profile, *cluster/runtime* settings are absent entirely.

### 6.2 Board picker — photo + wiring diagram, for visual confirmation

The picker is the heart of the page. ESP Web Tools auto-detects the **chip
family** over Web Serial; the page narrows the list to that chip, and for each
supported board shows:

- a **photo of the board** — so you can confirm "yes, that's the one in my hand"; and
- its **wiring diagram** — the DAC + encoder hookup for that board's default pins.

Both assets live per board under [`../esp32/devices/`](../../esp32/devices)
(`<board>.jpg` + `wiring-<board>.svg`) and are the same diagrams used elsewhere in
these docs. Picking a board loads its default pin map; you flash, set Wi-Fi, hit
**Test tone**, reboot — with the wiring picture right there to check against.

### 6.3 "Bring your own pins" — the escape hatch

For a breadboard build, a board we don't ship a profile for, or non-standard
wiring, the picker offers **Generic / bring-your-own-pins**: choose the chip
family, then set the I2S + encoder pins by hand (validated in firmware). No
photo/diagram — this is the power-user path; the named-board path is the front
door for everyone else.

### 6.4 USB transport is a board fact, not a question

Whether a board flashes over a **USB-UART bridge** (CH340/CH341/CP2102 → a
`ttyUSB` port, may need a host driver, **survives a crash/reset** so logs keep
streaming) or **native USB** (USB-Serial-JTAG / OTG-CDC → a driverless `ttyACM`
port that **re-enumerates** on reset) is fixed by the board (see the matrix). The
user never picks it; the page just shows the right conditional hint:

- bridge boards → "you may need the CH340/CP210x driver";
- native-USB boards → "the port disappears and comes back after reboot/flash — click reconnect."

For debugging, the bridge boards (the WROVER DevKitC) are nicer (persistent port
for panic backtraces); for end users, native-USB boards (S3-Zero) are nicer (one
driverless cable). First flash may need download mode — hold **BOOT**, tap
**RESET**, release BOOT; the installer instructs this.

### 6.5 Config-over-serial protocol

Line-delimited JSON on the console, framed `\n` — the same protocol whether the
panel or a script drives it. Validation (pin ranges, conflicts) is in firmware;
bad config is rejected, never half-applied.

```
→  {"cmd":"get"}
←  {"ok":true,"cfg":{"i2s_bclk":36,"i2s_lrck":35,...,"disc_mode":0}}
→  {"cmd":"set","cfg":{"wifi_ssid":"...","wifi_pass":"...","name":"kitchen"}}
←  {"ok":true}                          # validated + written to NVS
→  {"cmd":"set","cfg":{"i2s_dout":37,"enc_a":4}}   # BYO-pins path only
←  {"ok":true}
→  {"cmd":"test","what":"tone"}         # 1 kHz on the configured I2S — confirm wiring
←  {"ok":true}
→  {"cmd":"reboot"}
```

The USB console is the **wired** path (and the only way to set pins / DAC). A
deployed headless node can always be re-provisioned over USB or reset to defaults.

### 6.5a First-run captive portal (Tasmota-style)

For over-the-air first-run setup, the node also runs a **device-hosted captive
portal** (`main/provision.c`). It comes up in two cases:

- **Unprovisioned** — no `wifi_ssid` in NVS; or
- **Can't connect** — creds exist but no IP within
  `CONFIG_ENSEMBLE_STA_CONNECT_TIMEOUT_MS` (default 30 s), e.g. the stored AP is gone.

It opens an **open AP** named `ensemble-<first-4-hex-of-node-id>` (matching the mDNS
hostname), runs in **AP+STA** so the page can scan, and serves:

- `GET /` — a small offline form: Wi-Fi network (a datalist populated from `/scan`,
  or type it), password, speaker name (prefilled);
- `GET /scan` — JSON of nearby APs (`esp_wifi_scan`);
- `POST /save` — validates + writes creds/name to NVS (same
  `config_validate`→`config_save` path as the console) and reboots into STA;
- a catch-all **302 → `http://192.168.4.1/`** plus a wildcard **DNS responder** so
  the OS "sign-in" sheet pops up automatically.

The portal lives `CONFIG_ENSEMBLE_PORTAL_TIMEOUT_MS` (default **10 min**), then tears
itself down and the node goes **inert** — no reboot, no retry — until it is
power-cycled. The USB console stays live the whole time as the wired fallback. This
realigns with §5 ("an unprovisioned board boots into AP/provisioning mode").

### 6.6 Flashing flow (user's view)
1. Plug the board in via USB-C; open the flasher in Chrome/Edge.
2. **Pick your board** — confirm against the photo; the wiring diagram shows the hookup.
3. **Install** → ESP Web Tools flashes the matching merged image (BOOT-button hint if needed).
4. **Set Wi-Fi** (and an optional name).
5. **Test tone** to verify the DAC wiring → **Reboot**.
6. The node joins the LAN, the cluster discovers it, and it's assignable to any group.

---

## 7. Build, partitions, OTA
- **ESP-IDF 5.x**; pick a target with `./build.sh <board>` (or `idf.py
  -DBOARD=<board> build`) per [Boards & build targets](#boards--build-targets).
  Each board build merges to one `ensemble-fw-<board>.bin` for the flasher
  manifest; CI produces them alongside the Go release.
- Partition table: NVS + otadata + 2 OTA slots (`esp32/partitions.csv`). App-only
  flash at `0x20000` preserves NVS (node id + Wi-Fi); a merged flash at `0x0`
  wipes it.
- **OTA** (optional, later): check a release URL on boot; the flasher can also
  push an update over serial.

---

## 8. Conformance

The firmware must pass the same behavioral bar as the reference player
(`cmd/player`) and the e2e conformance leg (`scripts/e2e.sh` step 11b):
subscribe to a live group, play in sync (no dropouts, sample-aligned within the
buffer), survive `RECONFIG`/master-change, and stay out of cluster membership.
A bench check: run a software member and the ESP node in the same group on the
same track; both speakers should be phase-aligned (use [`calibration.md`](calibration.md)
to remove the fixed per-device offset).

## 9. Scope / limits (v1)
Receive-only and non-mastering (no library, never sources); membership via mDNS +
the v2 control plane (no gossip, no HTTP on the MCU — a master discovers, represents,
and drives it, §1b); opus on Wi-Fi; one group at a time; 2.4 GHz Wi-Fi. No TLS/auth
(trusted LAN, matching the rest of ensemble). Mic/voice and BLE are out of scope (a
player is receive-only over Wi-Fi).
