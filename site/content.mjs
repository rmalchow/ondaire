// All marketing copy for the ensemble site lives here. Edit this file and run
// `node build.mjs` (or `npm run build`) to regenerate site/dist/. No other file
// needs to change to update the words on the page.

const REPO = "https://gitlab.rand0m.me/share/ensemble";
const GITHUB = "https://github.com/rmalchow/ensemble";
const RELEASES = `${REPO}/-/releases`;
const DOC = (f) => `${REPO}/-/blob/main/docs/user/${f}`;
const GUIDE = DOC("README.md");

export const content = {
  meta: {
    title: "ensemble — synchronized multiroom audio",
    description:
      "Open-source, self-hosted multiroom audio. Drop one small binary on each device — they find each other and play in perfect sync. No cloud, no accounts, no config.",
    url: "",
    themeColor: "#0a0c10",
  },

  brand: { name: "ensemble" },

  // Header nav is a trimmed shortlist — every section still exists on the page.
  // (The "Flash a node" sub-page is still built — flash.html — but intentionally
  // UNLINKED until the firmware passes the conformance bar; see docs/developer/esp32.md.)
  nav: [
    { label: "Why", href: "#why" },
    { label: "Screenshots", href: "#screens" },
    { label: "Quickstart", href: "#quickstart" },
    { label: "Under the hood", href: "#tech" },
    { label: "GitHub", href: GITHUB, icon: "github" },
  ],

  hero: {
    eyebrow: "Open-source multiroom audio",
    // each line is rendered on its own row
    title: ["Every room.", "One sound.", "Zero fuss."],
    lede:
      "ensemble is self-hosted, synchronized audio for your whole home. Drop one small binary on each device — they discover each other and play in perfect sync. No cloud, no accounts, no config files.",
    primary: { label: "Get it", href: "download.html" },
    secondary: { label: "View source", href: GITHUB },
    snippet: { cmd: "./ensemble", caption: "That’s the whole setup." },
    shot: {
      src: "assets/img/overview.png",
      alt: "The ensemble web app on a phone: a playing room group with cover art, now-playing track, group volume and per-speaker volumes.",
    },
  },

  why: {
    eyebrow: "Why ensemble",
    title: "Built to disappear into your home.",
    features: [
      {
        n: "01",
        tag: "simple",
        title: "No moving parts",
        body:
          "One static binary. No config files, no database, no message broker, no account. Copy it to each device, run it, done — multiroom audio without the weekend project.",
      },
      {
        n: "02",
        tag: "automatic",
        title: "Sets itself up",
        body:
          "Nodes find each other over your LAN automatically. Ports, audio devices and host capabilities are detected per machine, with sensible defaults everywhere. First run just works; tune later if you want.",
      },
      {
        n: "03",
        tag: "open",
        title: "Yours to keep",
        body:
          "Free and open source, end to end. Pure-Go builds for Linux on amd64, arm64 and the Pi Zero. Optional Spotify Connect built in. No telemetry, no lock-in — your audio stays on your network.",
      },
      {
        n: "04",
        tag: "built-in ui",
        title: "A UI in every node",
        body:
          "Every node serves the same web app and proxies to the rest. Group rooms, set volumes, browse your library and see what’s playing — from any phone or browser, with nothing to install.",
      },
    ],
  },

  screens: {
    eyebrow: "A Look Around",
    title: "One app, the whole house.",
    items: [
      {
        src: "assets/img/overview.png",
        alt: "Rooms overview with a playing group, now-playing bar and volumes.",
        kicker: "Now playing",
        title: "See and steer every room",
        body:
          "Group devices into rooms, watch the current track with cover art and position, and balance volume per room and per speaker.",
      },
      {
        src: "assets/img/room-expanded.png",
        alt: "A selected room revealing the add-players roster and the media browser.",
        kicker: "In-line control",
        title: "Add players, pick music",
        body:
          "Select a room to reveal its roster and the master’s library. Drop in a speaker, browse folders, and press play — no menus to dig through.",
      },
      {
        src: "assets/img/queue.png",
        alt: "A playing room with a Next button above an ‘Up next’ list of six queued tracks, each removable.",
        kicker: "Shared queue",
        title: "Everyone adds what’s next",
        body:
          "Queue up a track — or a whole folder — and it plays gaplessly, back to back. The queue belongs to the room, not to one phone: anyone in the house can add what they want to hear, skip ahead, or pull a track. Titles come straight from your files’ tags.",
      },
      {
        src: "assets/img/spotify-endpoints.png",
        alt: "The Spotify endpoints editor with a default device and a custom preset of players.",
        kicker: "Spotify Connect",
        title: "Multi-room Spotify",
        body:
          "Expose named Connect devices that each map to a set of speakers. Pick one in the Spotify app and ensemble forms that group and plays — built in via go-librespot.",
      },
      {
        src: "assets/img/nodes.png",
        alt: "The Nodes page with per-device sections for features and settings.",
        kicker: "Per node",
        title: "Tune every device",
        body:
          "Volume, fine hardware-delay alignment, output device and feature toggles — for each node on your network, from the same page.",
      },
    ],
  },

  // The home-page Quickstart: the three-step gist + a Download CTA. The detailed
  // install one-liner and the config-flag reference now live on the download page.
  quickstart: {
    eyebrow: "Quickstart",
    title: "Three steps, then it’s just music.",
    steps: [
      {
        n: "1",
        title: "Run a node",
        body:
          "Start ensemble on each device with a speaker — a Raspberry Pi, an old laptop, your NAS. One command, no flags required.",
      },
      {
        n: "2",
        title: "They find each other",
        body:
          "Nodes form a cluster over mDNS automatically. No central server, no broker, no setup — peers appear within seconds.",
      },
      {
        n: "3",
        title: "Group and play",
        body:
          "Open the built-in UI, group devices into rooms, and play. A shared clock keeps every speaker aligned to the millisecond.",
      },
    ],
    cta: {
      text: "That’s the whole idea. Grab a build for your hardware and you’ll have the house playing in sync in a few minutes.",
      label: "Download",
      href: "download.html",
    },
  },

  tech: {
    eyebrow: "Under The Hood",
    title: "Playing in sync is hard. ensemble does the hard part.",
    intro:
      "Two speakers playing the same instant over flaky Wi-Fi means fighting both the network and physics. Four problems, four fixes:",
    items: [
      {
        tag: "network jitter",
        problem:
          "Wi-Fi delivers packets in bursts and at uneven intervals — played naively, audio stutters and rooms drift apart.",
        solution:
          "Every frame is stamped with a presentation time and played at that exact deadline against a shared clock. A small per-group playout buffer absorbs the jitter, so output stays smooth and every room hits the same instant.",
      },
      {
        tag: "packet loss",
        problem:
          "On a busy network UDP datagrams simply vanish, and a dropped frame is an audible click or gap.",
        solution:
          "Audio is Opus-coded so each frame fits one small packet (no fragile IP fragmentation), and the master sends forward-error-correction parity alongside it, so most lost frames are rebuilt with no retransmit. A TCP transport is one toggle away when you'd rather trade a little latency for certainty.",
      },
      {
        tag: "system clock drift",
        problem:
          "No two devices agree on what time it is — their clocks start at different offsets and tick at slightly different rates.",
        solution:
          "One node is the time reference. Each player continuously measures its offset and round-trip to that master and translates “play at T” into its own local time, so the same frame lands at the same real-world instant on every speaker.",
      },
      {
        tag: "DAC clock drift",
        problem:
          "Even with perfect timing, no two sound cards sample at exactly 48 kHz — a few parts-per-million apart, rooms slide out of phase over a long track.",
        solution:
          "A continuous rate servo watches how fast each DAC actually drains its buffer versus the master timeline and resamples by a micro-correction to match. Rooms stay phase-locked for hours, not just for the first minute.",
      },
    ],
  },

  // Measured-coherence proof. Branded graphs (rendered bare by tools/calib/, the
  // headline + judgement live here as brand-font text). Honest, not just flattering.
  proof: {
    eyebrow: "Measured, not promised",
    title: "We put a microphone in the room.",
    intro:
      "Anyone can claim “perfect sync.” So we recorded two Raspberry-Pi speakers with a single microphone and measured the real acoustic gap between them — ten minutes, every burst, warts and all.",
    items: [
      {
        src: "assets/img/sync_time.png",
        alt: "Inter-speaker offset over a ten-minute run: a flat line hugging zero, the bulk of bursts inside ±0.1 ms and 99% inside ±0.4 ms.",
        kicker: "Inter-speaker sync",
        metric: "84 µs",
        title: "Locked, the whole session",
        body:
          "Across a ten-minute run the two speakers held a median 84 µs apart — flat, burst after burst, not the drift-and-snap of a loop fighting itself. Your ears fuse two arrivals into one source up to roughly five milliseconds (the precedence effect), so this sits deep inside “one sound.” Honest read: the graph has the slow sound-card warm-up drift removed — the master’s cross-room equalization tracks that out — so what you see is the moment-to-moment sync.",
      },
      {
        src: "assets/img/sync_cdf.png",
        alt: "Cumulative distribution of the inter-speaker offset: 50% of bursts within 84 µs, 95% within 0.31 ms, 99.5% within 0.44 ms, none past half a millisecond.",
        kicker: "How close, how often",
        metric: "99.5% < 0.44 ms",
        title: "Sub-half-millisecond, by the numbers",
        body:
          "The whole distribution, not a cherry-picked peak: half the bursts land within 84 µs, 95% within 0.31 ms, 99.5% within 0.44 ms — and nothing crosses half a millisecond. Honest read: this is what a single microphone hears, so it already includes the room and the mic’s own ~150 µs of noise; the electrical sync between the cards is at least this tight, not looser.",
      },
    ],
  },

  // Tongue-in-cheek "testimonials". Every quote is invented; the disclaimer makes
  // that unmistakable. Portraits are from Wikimedia Commons, vendored locally.
  testimonials: {
    eyebrow: "Rave reviews",
    title: "The greats can’t stop talking about it.",
    note: "Every quote below is entirely fictional — they never said any of this, and several of them predate electricity. Portraits via Wikimedia Commons.",
    items: [
      {
        name: "Ludwig van Beethoven",
        role: "Composer · 1770–1827",
        quote:
          "I could not hear a single one of my rooms — yet they played as one. Magnificent. I was, however, not consulted.",
        img: "assets/img/testimonials/beethoven.jpg",
      },
      {
        name: "Wolfgang Amadeus Mozart",
        role: "Composer · 1756–1791",
        quote:
          "Setup took less time than a cadenza. One file, no fuss — even a prodigy could manage it.",
        img: "assets/img/testimonials/mozart.jpg",
      },
      {
        name: "Johann Sebastian Bach",
        role: "Composer · 1685–1750",
        quote:
          "Every voice entering at precisely the right instant, in every room at once. Counterpoint, but for speakers.",
        img: "assets/img/testimonials/bach.jpg",
      },
      {
        name: "Miles Davis",
        role: "Jazz · 1926–1991",
        quote:
          "It’s not the notes you sync, it’s the ones you don’t. ensemble gets the silence right in all five rooms.",
        img: "assets/img/testimonials/miles-davis.png",
      },
      {
        name: "Ella Fitzgerald",
        role: "Jazz vocalist · 1917–1996",
        quote:
          "No cloud, no accounts, no scat about subscriptions. Put it on and the whole house swings.",
        img: "assets/img/testimonials/ella-fitzgerald.jpg",
      },
      {
        name: "Freddie Mercury",
        role: "Rock · 1946–1991",
        quote:
          "I want it all, I want it all — and I want it in every room. Darling, it delivered.",
        img: "assets/img/testimonials/freddie-mercury.jpg",
      },
      {
        name: "Jimi Hendrix",
        role: "Rock guitarist · 1942–1970",
        quote:
          "’Scuse me while I sync the sky. Kitchen, hallway, garage — all phase-locked. Far out.",
        img: "assets/img/testimonials/jimi-hendrix.jpg",
      },
      {
        name: "Prince",
        role: "Pop · 1958–2016",
        quote:
          "Dearly beloved, we are gathered to play one song in every room. No latency, no cloud. Just the music.",
        img: "assets/img/testimonials/prince.png",
      },
    ],
  },

  cta: {
    title: "Bring your speakers together.",
    body:
      "Grab a build for your device, or read the user guide to see the whole app first.",
    primary: { label: "Get it", href: "download.html" },
    secondary: { label: "Read the guide", href: GUIDE },
  },

  // The separate download page (download.html). Each option's `file` is resolved
  // at build time: build.mjs hashes the staged binary (src/assets/downloads/) and
  // fills in its SHA-256 + size. `docker` options carry a pull command instead.
  download: {
    eyebrow: "Download",
    title: "Get ensemble for your hardware.",
    intro:
      "One small, static binary per device — pure Go, no runtime, no dependencies. Each archive is the build attached to the matching tagged release: verify its SHA-256, unpack it, and run ./ensemble. Prefer containers? Pull the master image with Spotify Connect built in.",
    // Teaser for the (in-progress) hardware-player support. This block will
    // eventually be the entry point to the browser flasher (flash.html); no link
    // yet — the firmware isn't conformant. See docs/developer/esp32.md + docs/developer/player-protocol.md.
    esp32: {
      badge: "Coming soon",
      title: "ESP32 players — support in progress",
      body:
        "Turn a PSRAM-equipped ESP32 + an I2S DAC into a real ensemble player: it shows up in the cluster, joins any group, and plays in lock-step like every other room — flashed straight from your browser, no toolchain. Supported boards are PSRAM ESP32s: ESP32-S3 (e.g. the ESP32-S3-DevKitC-1, or the tiny Waveshare ESP32-S3-Zero) and the classic ESP32-WROVER. The browser flasher will land right here soon.",
    },
    // Guided installer (scripts/get.sh) — the fastest path onto a Linux box.
    // Rendered after the ESP32 teaser, before the per-arch download cards.
    installer: {
      title: "Installer",
      body:
        "The quickest way onto a Linux box: a guided one-liner. It detects your OS and CPU, downloads the matching ensemble build, then walks you through the optional extras — Spotify Connect (go-librespot) and a boot-time systemd service — and sets them up. Installs into /usr/local/lib/ensemble; re-run any time to update.",
      code: "curl -fsSL https://ensemble.rand0m.me/get.sh | sudo bash",
      // Expandable, annotated walkthrough of scripts/get.sh — its real flow with
      // section labels + explanatory comments (not the verbatim 256 lines).
      walkthrough: {
        summary: "What the script does, step by step",
        hrefLabel: "Read the full get.sh",
        href: `${REPO}/-/blob/main/scripts/get.sh`,
        script: `#!/usr/bin/env bash
#   curl -fsSL https://ensemble.rand0m.me/get.sh | sudo bash
set -euo pipefail

# ── 1. Pre-flight ──────────────────────────────────────────────
# Must run as root, on Linux, with tar present.
[ "$(id -u)" = 0 ] || err "run with sudo"

# ── 2. Detect the CPU and pick the matching 64-bit build ───────
case "$(uname -m)" in
  x86_64|amd64)   ARCH=amd64 ;;
  aarch64|arm64)  ARCH=arm64 ;;          # Raspberry Pi OS 64-bit
  *) err "unsupported arch (32-bit is no longer supported)" ;;
esac

# ── 3. Download + install ──────────────────────────────────────
# Binary lands in /usr/local/lib/ensemble, symlinked onto your PATH.
fetch "$BASE/assets/downloads/ensemble-linux-$ARCH.tar.gz" | tar -xz
install -m755 ensemble /usr/local/lib/ensemble/ensemble
ln -sf  /usr/local/lib/ensemble/ensemble /usr/local/bin/ensemble

# ── 4. Choose a role (prompts read the terminal, so they work
#       even under  curl | bash) ─────────────────────────────────
if ask "Run the web UI on this node?"; then
  ROLE="master,playback"      # serves the UI, gossips, can play audio
else
  ROLE="playback"             # receive-only, driven by a master
fi

# ── 5. Spotify Connect — masters only, optional ────────────────
# Fetches the matching go-librespot build alongside ensemble.
[ "$ROLE" != playback ] && ask "Install Spotify Connect?" && install_go_librespot

# ── 6. Boot service — optional ─────────────────────────────────
# Writes /etc/systemd/system/ensemble.service and enables it, so
# ensemble starts at boot and restarts on failure.
ask "Start ensemble at boot (systemd)?" && install_systemd_unit

# ── 7. Appliance hardening — optional, for an SD-card Pi ───────
# Console-only (frees the audio card from the desktop), trims extra
# services, sends logs to RAM, disables swap.
ask "Harden as a headless audio appliance?" && harden_appliance

echo "ready — open the web UI at  http://<this-host>:8080"`,
      },
    },
    options: [
      {
        name: "Raspberry Pi — 64-bit",
        rec: "Raspberry Pi OS Lite (64-bit) on a Pi 3 / 4 / 5 or Zero 2, or any other arm64 Linux. 32-bit Pi OS is no longer supported.",
        arch: "linux · arm64",
        logos: ["raspberrypi"],
        file: "assets/downloads/ensemble-linux-arm64.tar.gz",
        note:
          "Heads-up: Opus playback loads <strong>libopus</strong> at runtime. The Desktop image already has it; on a minimal Lite install that lacks it, add it with <code>sudo apt install libopus0</code> (uncompressed PCM works without it). Audio hardware that needs third-party drivers is out of ensemble's scope — get the card working in Linux first.",
      },
      {
        name: "PC or server — x86-64",
        rec: "Any modern Linux distribution on a 64-bit Intel/AMD machine.",
        arch: "linux · amd64",
        logos: ["fedora", "ubuntu", "debian", "arch", "manjaro"],
        file: "assets/downloads/ensemble-linux-amd64.tar.gz",
      },
      {
        name: "Docker",
        rec: "For a NAS or server: runs a master — mount your music library and serve it to your players, with Spotify Connect (go-librespot) built in. Multi-arch: amd64 · arm64.",
        arch: "container image",
        logos: ["docker"],
        docker: [
          "docker run -d --network host \\",
          "  -v /srv/music:/media:ro \\",
          "  -v ensemble-data:/data \\",
          "  harbor.rand0m.me/public/ensemble:latest --name living-room",
        ].join("\n"),
        // First paragraph: trusted author HTML, rendered as plain body text.
        body:
          "The image runs the <strong>master role only</strong> by default — it sources audio (your library + Spotify Connect) and controls the cluster, but does not play locally. Reaching host audio hardware (sound cards, USB DACs, I2S) from a container isn't reliable and is <strong>out of scope</strong>: the actual output happens on your players (the Pi binary, or an ESP32 node). Mount your library read-only on <code>/media</code>; mutable state lives on <code>/data</code>. You can change any default with the <code>ENSEMBLE_*</code> environment variables — see the config flags below.",
        // The one genuine gotcha, kept as a callout.
        note:
          "<strong><code>--network host</code> is required.</strong> Players discover the master over mDNS and go-librespot advertises Spotify Connect over zeroconf — multicast doesn't cross Docker's bridge network (and ports bind-or-increment, so there's nothing to publish).",
      },
    ],
    // Common config flags — rendered as its own block UNDER the download cards.
    flags: {
      title: "Common config flags",
      intro:
        "Almost every flag has an ENSEMBLE_* environment equivalent (handy in Docker); all are optional, with sensible defaults. Ports left at their default bind-or-increment (run several nodes on one box); a port you set explicitly is pinned (binds exactly or exits).",
      // Rendered as a param · env var · default · description table.
      cols: ["param", "env var", "default", "description"],
      params: [
        { param: "--name <name>", env: "—", def: "node id", what: "display name + Spotify device name (first start only)" },
        { param: "--role <role>", env: "ENSEMBLE_ROLE", def: "both", what: "master | playback | master,playback" },
        { param: "--media <dir>", env: "ENSEMBLE_MEDIA_DIR", def: "<data>/media", what: "library directory, browsed recursively" },
        { param: "--data <dir>", env: "ENSEMBLE_DATA_DIR", def: "./data", what: "node.json, cluster state, Spotify creds" },
        { param: "--http-port <n>", env: "ENSEMBLE_HTTP_PORT", def: "8080", what: "UI + REST API + WebSocket + node proxy" },
        { param: "--output <spec>", env: "ENSEMBLE_OUTPUT", def: "auto", what: "alsa · exec · null · file:<path>" },
      ],
      doc: { label: "Full configuration reference", href: DOC("config-reference.md") },
    },
    links: [
      {
        desc: "Looking for an older version, the changelog, or release notes?",
        label: "Other versions",
        href: RELEASES,
      },
      {
        desc: "New here and need help getting a node running?",
        label: "How to run it",
        href: DOC("running.md"),
      },
    ],
  },

  // The browser web-flasher page (flash.html). ESP Web Tools detects the chip
  // and flashes the matching merged firmware from `firmware.builds`; the custom
  // panel then provisions Wi-Fi + I2S/encoder over Web Serial (no toolchain).
  flash: {
    eyebrow: "Build a node",
    title: "Flash a DIY speaker, right from your browser.",
    intro:
      "Turn an ESP32 + an I2S DAC into a real ensemble player — it shows up in the cluster, joins any group, and plays in lock-step like every other room. No toolchain, no app: plug it in over USB-C in Chrome or Edge, click Install, then set your Wi-Fi and pins. Receive-only, opus over Wi-Fi.",
    requirements:
      "Needs Chrome or Edge on desktop (Web Serial). Plug the board in via USB-C. First flash on an S2/S3 may need download mode: hold BOOT, tap RESET, release BOOT — the installer will tell you.",
    install: { label: "Install firmware", note: "ESP Web Tools picks the right build for your chip automatically." },
    bom: {
      title: "What you need",
      items: [
        "A PSRAM-equipped ESP32 board — an ESP32-S3 (DevKitC-1 or Waveshare S3-Zero) or a classic ESP32-WROVER.",
        "A PCM5102A I2S DAC (the common purple GY-PCM5102 module).",
        "A KY-040 / EC11 rotary encoder for local volume (optional).",
        "Wiring + pinouts live in the repo under esp32/devices/.",
      ],
    },
    steps: [
      { n: "1", title: "Plug in & install", body: "Connect the board over USB-C, click Install, and wait for the flash to finish." },
      { n: "2", title: "Set Wi-Fi", body: "Connect over serial and enter your 2.4 GHz network — credentials are written straight to the device." },
      { n: "3", title: "Confirm wiring", body: "Set the I2S and encoder pins (defaults match the wiring guide), then hit Test tone to verify the DAC." },
      { n: "4", title: "Reboot & play", body: "The node joins the LAN, the cluster discovers it, and it’s assignable to any group. Turn the knob for volume." },
    ],
    docHref: `${REPO}/-/blob/main/docs/developer/esp32.md`,
    docLabel: "Firmware & hardware guide",
  },

  // Firmware builds offered by the flasher. Each `file` is resolved at build
  // time (build.mjs resolveFirmware) into the ESP Web Tools manifest + SHA-256.
  firmware: {
    manifestName: "ensemble player",
    builds: [
      { chipFamily: "ESP32-S3", label: "ESP32-S3", note: "ESP32-S3-WROOM-1 / DevKitC-1 (PSRAM)", file: "assets/firmware/ensemble-fw-esp32s3.bin" },
    ],
  },

  footer: {
    note: "Pure Go + Svelte. Runs on a Raspberry Pi. No cloud, no telemetry.",
    links: [
      { label: "Source", href: REPO },
      { label: "Releases", href: RELEASES },
      { label: "User guide", href: GUIDE },
    ],
  },
};
