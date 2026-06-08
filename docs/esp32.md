# ESP32-S2 dumb node

A hardware **node**: a tiny receive-only ensemble speaker built on an
**ESP32-S2-WROOM** + an I2S DAC. It is **visible and assignable** in the cluster
(it shows up in the UI, you can drop it into any group, set its volume and
delay, and auto-calibrate it) but it is a **thin** node — it never sources
audio (no library, can't become master) and it does **not** run gossip. The
audio wire protocol it speaks is specified in [`DUMB-CLIENT.md`](DUMB-CLIENT.md);
this document is the firmware + hardware + membership + provisioning design.
Status: **design spec** (not yet implemented).

> Companion board: the [`kicad/` amp board](../kicad) is an ESP32-S3 +
> PCM5102A + TPA3118D2 reference. The firmware here targets the **S2-WROOM**
> (single-core, native USB — ideal for the web flasher); it is otherwise
> identical in concept and shares the I2S/DAC design.

---

## 1. What it must do (recap)

From `DUMB-CLIENT.md`, a conforming node:

1. **Discovers** the group master — either a fixed master IP/port from config,
   or by polling any node's `GET /api/cluster` over HTTP and resolving its
   group's `master` → `sourcePort`/`streamPort`.
2. **Subscribes** to the master's `SOURCE_PORT` (UDP): `HELLO` (prime-me),
   re-`HELLO` keepalive every 5 s, `RESTART` on >2 s starvation, honor
   `RECONFIG`.
3. **Clock-syncs** to the master's `STREAM_PORT` (UDP) at 1 Hz: 4-timestamp
   NTP exchange, median-of-best-RTT offset, against a local monotonic clock.
4. **Plays out**: jitter-buffer received frames and emit each at
   `pts − offset + bufferMs` on the local clock; silence on gaps.
5. **Decodes opus** (the default group codec; PCM datagrams fragment on Wi-Fi —
   §1 of DUMB-CLIENT.md). Opus is effectively mandatory on Wi-Fi hardware.

Everything below is *how* an S2-WROOM does that, plus membership, local volume,
and a browser-based flasher/provisioner.

---

## 1b. Membership — a "thin" node (no gossip on the MCU)

A dumb node should be **visible and assignable**, not a hidden fixed-IP speaker.
But gossip in ensemble *is* hashicorp/memberlist (SWIM, TCP push/pull, msgpack,
LWW-doc replication) — too heavy to reimplement on a single-core S2, and
unnecessary. Instead the MCU runs **mDNS + a tiny HTTP API**, and the full
nodes represent it in the replicated doc:

```
   ESP node                         any full node
   ────────                         ─────────────
   mDNS advertise  ───────────────▶ discovers _ensemble._tcp TXT (thin=1)
   _ensemble._tcp                   │
   thin=1, id, ports                ▼  injects a thin node-record into the
                                    │  gossip doc; refreshes lastSeen by
   GET  /api/status   ◀────────────┘  HTTP-health-polling the ESP node
   PATCH /api/node    ◀── volume / outputDelayMs / name   (UI, calibration)
   POST /api/follow|/unfollow ◀── group assignment        (UI, takeover)
```

What this buys, for ~5 HTTP endpoints + mDNS on the chip (no SWIM, no msgpack,
no doc merge):

- **Visible** in `/api/cluster` and the Nodes UI.
- **Assignable** — `follow`/`unfollow` hit the node's own HTTP (direct or via
  the proxy), so the UI's *Add node…* / *Leave* and play-from-node takeover work.
- **Controllable & calibratable** — `outputDelayMs`, `volume`, `name` are
  `PATCH`ed to its HTTP, so the UI sliders **and** the auto-calibration
  ([`calibrate.md`](calibrate.md)) treat it exactly like a software node.
- **Non-mastering** — no media library, never runs a source; a takeover/`play`
  is refused (`ErrNotMaster`-equivalent). By default it's a solo group of one
  that plays nothing until it follows someone.

Server-side support needed (small Go change, tracked separately): a `thin` node
kind whose record is **HTTP-health-polled** by full nodes instead of
gossip-tracked, and which ages out when no full node can reach it. The MCU's
`following` lives in its own NVS/HTTP state, gossiped on its behalf, so group
membership derives normally (§5 of the spec).

> **Fully-dumb fallback** (`disc_mode=1`, fixed master IP, no HTTP/mDNS): the
> non-cluster mode from `DUMB-CLIENT.md` — invisible, pinned to one master, not
> calibratable through the UI. Use only on segmented networks or for bring-up.

---

## 2. Hardware

### 2.1 Bill of materials (minimum)
- **ESP32-S2-WROOM** module (single-core Xtensa LX7 @ 240 MHz, 320 KB SRAM,
  ≥4 MB flash, **no PSRAM**, native USB-OTG, 2.4 GHz Wi-Fi).
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

### 3.1 Memory budget (320 KB SRAM, no PSRAM)
- Jitter buffer: 200 ms PCM @ 48 k stereo s16 ≈ **38 KB** (ring of ~10×20 ms
  frames + a little slack).
- Opus decoder state + scratch ≈ **30 KB**.
- Wi-Fi/lwIP ≈ **50 KB**; I2S DMA ≈ **8 KB**; tasks/stacks ≈ **20 KB**.
- Comfortable headroom; PSRAM not required.

### 3.2 Opus decode
Default group codec. Decode each 20 ms / 48 kHz / stereo Opus packet to PCM
before the jitter buffer. libopus decode-only on a single 240 MHz LX7 is a small
fraction of a core; it is the right choice because a 20 ms Opus packet (~320 B)
is one unfragmented datagram on Wi-Fi. (PCM mode is accepted for wired/bench use
but discouraged — see DUMB-CLIENT.md.)

### 3.3 Clock & the rate servo — done in hardware
The server's rate servo resamples in software; the ESP32 does it for free in
the **I2S clock**. The I2S sample rate is derived from the **APLL**, whose
fractional divider can be nudged in ~ppm steps. So instead of resampling:

- Drive the **jitter-buffer fill level** (or the playout phase error) to a
  setpoint with a slow proportional controller.
- The controller output trims the APLL sample rate by ±N ppm.
- Net effect: the DAC consumes at exactly the master's effective rate; the
  buffer neither drifts empty nor full. Inaudible, no software resampling.

This mirrors the server-side queue servo (`internal/sink/servo.go`) but actuates
the real clock. Clamp to ±300 ppm; smooth heavily (Wi-Fi clock jitter).

### 3.4 Playout
`esp_timer` (64-bit µs since boot) is the local monotonic clock. The clock
follower yields `offset`; the audio task schedules frame `pts` for emission at
`MasterToLocal(pts) + bufferMs`, writing into the I2S DMA ring so audio leaves
the DAC at that instant. Missing frames (after the small reorder/FEC window) play
silence. Apply local volume (gain or DAC register) just before I2S.

---

## 4. Local volume — rotary encoder (and optional I2C)

- **Quadrature decode** via the S2 **PCNT** peripheral (or A/B GPIO ISR) with
  debounce; one detent = ±1 step. The push switch toggles **mute**.
- **Apply** the volume as either: (a) a digital gain on the PCM (software,
  any DAC), or (b) the DAC's hardware volume register over **I2C** (PCM5122) —
  cleaner, no bit loss. I2C is optional; without it, software gain is used.
- **Optional OLED** (I2C): show group name (from the discovery poll), volume %,
  sync state, and a buffer/health bar.
- **Cluster-visible volume**: because a thin node serves its own HTTP API
  (§1b), the knob and the UI/`PATCH /api/node {volume}` are the *same* value —
  the firmware applies it locally and reports it in `GET /api/status`, so the UI
  slider and the knob stay in sync. (In fully-dumb fallback mode it is
  local-only.)

---

## 5. Configuration (NVS)

All settings persist in NVS namespace `ensemble`. Defaults let an unprovisioned
board boot into AP/provisioning mode.

| Key | Type | Meaning |
|-----|------|---------|
| `wifi_ssid` / `wifi_pass` | str | Wi-Fi credentials (set via Improv) |
| `disc_mode` | u8 | `0`=mDNS auto, `1`=fixed master |
| `master_ip` / `source_port` / `stream_port` | str/u16 | fixed-master target |
| `group` | str | optional: which group/node name to follow when several exist |
| `codec` | u8 | `0`=opus (default), `1`=pcm |
| `buffer_ms` | u16 | playout buffer (default 150) |
| `i2s_bclk`/`i2s_lrck`/`i2s_dout`/`i2s_mclk` | u8 | I2S pins |
| `i2c_en`/`i2c_sda`/`i2c_scl` | u8 | optional I2C |
| `dac` | u8 | `0`=PCM5102A (sw gain), `1`=PCM5122 (I2C volume) |
| `enc_a`/`enc_b`/`enc_sw` | u8 | rotary encoder pins |
| `vol` | u8 | last volume 0–100 (restored on boot) |
| `name` | str | friendly label (logs / OLED) |

---

## 6. Web flasher & provisioner

A **static web page** (Chrome/Edge, Web Serial API) that flashes the firmware
and writes all of §5 over the USB cable — no toolchain, no app install. Hosted on
GitLab Pages (or any static host) and linked from releases.

### 6.1 Stack
- **[ESP Web Tools](https://esphome.github.io/esp-web-tools/)** —
  `<esp-web-install-button manifest="manifest.json">`. Flashes the merged
  firmware via the serial bootloader over native USB. (First flash: the S2 may
  need download mode — hold **BOOT**, tap **RESET**, release BOOT; the page
  instructs this.)
- **Improv-Serial** (built into ESP Web Tools; the S2 has **no BLE**, so use the
  serial variant) — the firmware implements Improv-Serial, so the page shows a
  **“Connect to Wi-Fi”** dialog right after flashing and writes `wifi_ssid/pass`.
- **Custom settings panel** — for the non-Wi-Fi config (I2S/I2C pins, encoder,
  DAC type, discovery mode, default volume) the page talks a tiny
  **JSON-over-serial** protocol to the firmware's USB-CDC console.

### 6.2 Config-over-serial protocol
Line-delimited JSON on the USB-CDC console, framed `\n`:

```
→  {"cmd":"get"}
←  {"ok":true,"cfg":{"i2s_bclk":36,"i2s_lrck":35,...,"disc_mode":0}}
→  {"cmd":"set","cfg":{"i2s_dout":37,"dac":1,"i2c_sda":8,"i2c_scl":9,"disc_mode":1,"master_ip":"192.168.1.10"}}
←  {"ok":true}                          # validated + written to NVS
→  {"cmd":"test","what":"tone"}         # 1 kHz on the configured I2S — confirm wiring
←  {"ok":true}
→  {"cmd":"reboot"}
```

The same protocol is exposed so anyone can script provisioning; the web page is
just a friendly front-end over it. Validation (pin ranges, conflicts) happens in
firmware; bad config is rejected, never half-applied.

### 6.3 Fallback: device-hosted config
If serial isn't available (already deployed, headless), the firmware also serves
the same settings form over HTTP: on boot without Wi-Fi it raises a SoftAP
`ensemble-setup-XXXX` with a captive portal; once on Wi-Fi, the page is reachable
at the device's IP. Same JSON schema, same validation.

### 6.4 Flashing flow (user's view)
1. Plug the board in via USB-C, open the flasher page in Chrome/Edge.
2. **Install** → ESP Web Tools flashes (with the BOOT-button hint if needed).
3. **Connect to Wi-Fi** (Improv-Serial dialog).
4. **Device settings** panel → pick DAC, set/confirm I2S + encoder pins,
   discovery mode → **Save** → **Test tone** to verify audio → **Reboot**.
5. The node joins the LAN, finds the group, and plays. Turn the knob for volume.

---

## 7. Build, partitions, OTA
- **ESP-IDF 5.x**, target `esp32s2`. `idf.py build` → merged `firmware.bin` for
  the manifest. CI can produce the flasher artifacts alongside the Go release.
- Partition table: factory + 2 OTA slots + NVS (+ optional `spiffs` for the
  config page assets).
- **OTA** (optional, v1.1): check a release URL on boot; the flasher page can
  also push an update over serial.

---

## 8. Conformance

The firmware must pass the same behavioral bar as the reference client
(`cmd/dumbclient`) and the e2e conformance leg (`scripts/e2e.sh` step 11b):
subscribe to a live group, play in sync (no dropouts, sample-aligned within the
buffer), survive `RECONFIG`/master-change, and stay out of cluster membership.
A bench check: run a software member and the ESP node in the same group on the
same track; both speakers should be phase-aligned (use [`calibrate.md`](calibrate.md)
to remove the fixed per-device offset).

## 9. Scope / limits (v1)
Receive-only and non-mastering (no library, never sources); thin membership via
mDNS + HTTP (no gossip on the MCU); opus on Wi-Fi; one group at a time; 2.4 GHz
Wi-Fi. Needs the server-side `thin` node-kind (§1b) to be visible/assignable;
without it, fall back to fixed-master fully-dumb mode. No TLS/auth (trusted LAN,
matching the rest of ensemble). Mic/voice and BLE are out of scope (the S2 has
no BLE).
