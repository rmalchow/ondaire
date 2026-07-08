// All marketing copy for the ondaire site lives here. Edit this file and run
// `node build.mjs` (or `npm run build`) to regenerate site/dist/. No other file
// needs to change to update the words on the page.

const REPO = "https://gitlab.rand0m.me/share/ondaire";
const GITHUB = "https://github.com/rmalchow/ondaire";
const RELEASES = `${REPO}/-/releases`;
const DOC = (f) => `${REPO}/-/blob/main/docs/user/${f}`;
const GUIDE = DOC("README.md");

export const content = {
  meta: {
    title: "ondaire — synchronized multiroom audio",
    description:
      "Open-source, self-hosted multiroom audio. Drop one small binary on each device — they find each other and play in perfect sync. No cloud, no accounts, no config.",
    url: "",
    themeColor: "#0a0c10",
  },

  brand: { name: "ondaire" },

  // Header nav now points at the three topic pages the home blocks link to, plus
  // the source. download.html is the CTA; the "Flash a node" page (flash.html) is
  // reached from install.html but stays out of the top nav until the firmware
  // passes the conformance bar (see docs/developer/esp32.md).
  nav: [
    { label: "Install", href: "install.html" },
    { label: "The UI", href: "ui.html" },
    { label: "How it works", href: "tech.html" },
    { label: "GitHub", href: GITHUB, icon: "github" },
  ],

  hero: {
    eyebrow: "Open-source multiroom audio",
    // each line is rendered on its own row
    title: ["Every room.", "One sound.", "Zero fuss."],
    lede:
      "ondaire is self-hosted, synchronized audio for your whole home. Drop one small binary on each device — they discover each other and play in perfect sync. No cloud, no accounts, no config files.",
    primary: { label: "Get it", href: "download.html" },
    secondary: { label: "View source", href: GITHUB },
    shot: {
      src: "assets/img/overview.png",
      alt: "The ondaire web app on a phone: a playing room group with cover art, now-playing track, group volume and per-speaker volumes.",
    },
  },

  // The home page is three prominent blocks — one per topic — each a teaser that
  // links to its own full page (install.html / ui.html / tech.html). One block
  // uses a faux-terminal mock instead of a screenshot (term:true).
  home: {
    blocks: [
      {
        kicker: "Install in minutes",
        title: "One binary. Run it.",
        body:
          "No config files, no database, no message broker, no account. Drop a single static binary on each device — a Raspberry Pi, an old laptop, your NAS — run it, and it just works. Prefer a guided one-liner, Docker, or a DIY ESP32 speaker? There’s a clean path for each.",
        cta: { label: "See all install options", href: "install.html" },
        // Rendered as a terminal window rather than a screenshot.
        term: [
          { p: "$", t: "./ondaire" },
          { c: "ondaire — node “living-room”" },
          { c: "mdns:  discovered 2 peers on the LAN" },
          { c: "http:  UI at http://living-room:8080" },
          { c: "audio: aligned to shared clock ✓" },
        ],
      },
      {
        kicker: "One app, the whole house",
        title: "A comprehensive UI in every node.",
        body:
          "Every node serves the same web app and proxies to the rest — open it from any phone or browser, with nothing to install. Group rooms, balance volume per speaker, browse your library, share a queue anyone can add to, watch live per-node stats, and reskin the whole thing with a single click.",
        cta: { label: "Tour the interface", href: "ui.html" },
        img: "assets/img/room-expanded.png",
        alt: "The ondaire web app showing a room’s player roster and the media browser.",
        // Portrait phone shot — frame it like the hero rather than full-bleed.
        phone: true,
      },
      {
        kicker: "Measured, not promised",
        title: "Measurably great sync.",
        body:
          "Anyone can claim “perfect sync.” We put a microphone in the room and recorded two speakers for ten minutes: a median 84 µs apart, with 99.5% of bursts inside half a millisecond. See how the network jitter, packet loss and clock drift get solved — with the graphs to back every claim.",
        cta: { label: "See how it works", href: "tech.html" },
        img: "assets/img/sync_time.png",
        alt: "Inter-speaker offset over a ten-minute run: a flat line hugging zero.",
      },
    ],
  },

  // The UI tour page (ui.html). It reuses screens.items, grouped by `page` into
  // the room page and the nodes page, then the themes carousel below. The copy
  // points out features rather than narrating the interface.
  ui: {
    eyebrow: "The interface",
    title: "One app, the whole house.",
    intro:
      "There’s no separate controller to install: every node serves the same web app and proxies to the rest, so any phone or browser on your network drives the whole house. Here are the pieces that matter.",
    rooms: { kicker: "The room page", title: "Group, play, and share the queue." },
    nodes: { kicker: "The nodes page", title: "See and tune every device." },
  },

  screens: {
    eyebrow: "A Look Around",
    title: "One app, the whole house.",
    items: [
      {
        page: "rooms",
        src: "assets/img/overview.png",
        alt: "Rooms overview with a playing group, now-playing bar and volumes.",
        kicker: "Grouping & now playing",
        title: "See and steer every room",
        body:
          "Group devices into rooms, watch the current track with cover art and position, and balance volume per room and per speaker — the whole house on one screen.",
      },
      {
        page: "rooms",
        src: "assets/img/room-expanded.png",
        alt: "A selected room revealing the add-players roster and the media browser.",
        kicker: "Media library",
        title: "Add players, pick music",
        body:
          "Select a room to reveal its roster and the master’s library. Drop in a speaker, browse folders, and press play — no menus to dig through.",
      },
      {
        page: "rooms",
        src: "assets/img/queue.png",
        alt: "A playing room with a Next button above an ‘Up next’ list of six queued tracks, each removable.",
        kicker: "Shared queue & player",
        title: "Everyone adds what’s next",
        body:
          "Queue up a track — or a whole folder — and it plays gaplessly, back to back. The queue belongs to the room, not to one phone: anyone in the house can add what they want to hear, skip ahead, or pull a track. Titles come straight from your files’ tags.",
      },
      {
        page: "rooms",
        src: "assets/img/spotify-endpoints.png",
        alt: "The Spotify endpoints editor with a default device and a custom preset of players.",
        kicker: "Spotify Connect",
        title: "Multi-room Spotify",
        body:
          "Expose named Connect devices that each map to a set of speakers. Pick one in the Spotify app and ondaire forms that group and plays — built in via go-librespot.",
      },
      {
        page: "nodes",
        src: "assets/img/nodes.png",
        alt: "The Nodes page with per-device sections for live stats, features and settings.",
        kicker: "Per-node stats & tuning",
        title: "Watch and tune every device",
        body:
          "Live per-node stats — sync health, buffer, link quality — alongside volume, fine hardware-delay alignment, output device and feature toggles, for every node on your network from the same page.",
      },
    ],
  },

  // Theme carousel — the control app ships several built-in looks; one click swaps
  // the whole UI. Screenshots are captured from the live cluster, one per theme.
  themes: {
    eyebrow: "Make it yours",
    title: "Pick a look. One click.",
    intro:
      "The control app is fully themed — a single switch re-skins everything, fonts and all. A few built in, from a warm studio console to a pixel-art throwback.",
    items: [
      { name: "mint", img: "assets/img/themes/mint.png", blurb: "The classic Ondaire identity." },
      { name: "studio", img: "assets/img/themes/studio.png", blurb: "Warm tube-amp dark on walnut veneer." },
      { name: "nocturne", img: "assets/img/themes/nocturne.png", blurb: "Indigo night with a drifting starfield." },
      { name: "paper", img: "assets/img/themes/paper.png", blurb: "Bright, editorial light mode." },
      { name: "8bit", img: "assets/img/themes/8bit.png", blurb: "Blocky, pixelated, NES-green." },
      { name: "xp", img: "assets/img/themes/xp.png", blurb: "Luna blue, beige windows, Bliss sky." },
    ],
  },

  // The install-methods page (install.html). Each method is a card: a heading,
  // explanatory body, an optional code block (copy:true makes it copyable), an
  // optional note callout, and an optional CTA/doc link. The actual binaries +
  // SHA-256s live on the separate download.html (linked from the first method).
  install: {
    eyebrow: "Install",
    title: "Run ondaire anywhere.",
    intro:
      "One small, static binary per device — pure Go, no runtime, no dependencies. Pick the path that fits your setup; every one ends with a node on your network in a few minutes.",
    methods: [
      {
        tag: "the basics",
        title: "Just run it",
        body:
          "Grab the build for your hardware, unpack it, and run it. No flags required — it picks sensible defaults, finds your audio device, and joins the cluster on its own. That’s the whole setup.",
        code: "tar -xzf ondaire-linux-arm64.tar.gz\n./ondaire",
        cta: { label: "Download a build", href: "download.html" },
      },
      {
        tag: "fastest",
        title: "Guided installer",
        body:
          "The quickest way onto a Linux box: a guided one-liner. It detects your OS and CPU, downloads the matching build, then walks you through the optional extras — Spotify Connect and a boot-time service — and sets them up. Installs into /usr/local/lib/ondaire; re-run any time to update.",
        code: "curl -fsSL https://ondaire.rand0m.me/get.sh | sudo bash",
        copy: true,
        doc: { label: "What the installer does", href: `${REPO}/-/blob/main/scripts/get.sh` },
      },
      {
        tag: "nas / server",
        title: "Docker",
        body:
          "Run a master on a NAS or server: mount your music library read-only and serve it to your players, with Spotify Connect built in. The container sources and controls audio; output happens on your players, not in the container.",
        code: [
          "docker run -d --network host \\",
          "  -v /srv/music:/media:ro \\",
          "  -v ondaire-data:/data \\",
          "  harbor.rand0m.me/public/ondaire:latest --name living-room",
        ].join("\n"),
        copy: true,
        note:
          "<strong><code>--network host</code> is required.</strong> Players discover the master over mDNS and go-librespot advertises Spotify Connect over zeroconf — multicast doesn’t cross Docker’s bridge network.",
      },
      {
        tag: "always on",
        title: "Start at boot (systemd)",
        body:
          "Want ondaire to come up with the machine and restart on failure? The guided installer offers to write and enable a systemd service for you — no hand-editing unit files. Choose “yes” when it asks, or set it up later following the user guide.",
        cta: { label: "Running it as a service", href: DOC("running.md") },
      },
      {
        tag: "diy",
        title: "ESP32 player",
        body:
          "Turn a PSRAM-equipped ESP32 + an I2S DAC into a real ondaire player: it shows up in the cluster, joins any group, and plays in lock-step like every other room — flashed straight from your browser, no toolchain. Tested on the ESP32-S3 Super Mini and Waveshare ESP32-S3-Zero with a PCM5102A DAC.",
        cta: { label: "Open the browser flasher", href: "flash.html" },
      },
      {
        tag: "smart home",
        title: "Home Assistant",
        body:
          "Already run Home Assistant? Add the ondaire integration: every room shows up as a media_player, and a custom Lovelace card mirrors the ondaire app — players, media, streams and the queue. Unzip into custom_components, restart, and you're in.",
        cta: { label: "Home Assistant integration", href: "home-assistant.html" },
      },
    ],
    // Closing links on install.html, mirroring the download-page link rows.
    links: [
      {
        desc: "Want the actual binaries, SHA-256 hashes, and the Docker image?",
        label: "Download builds",
        href: "download.html",
      },
      {
        desc: "New here and need a hand getting a node running?",
        label: "How to run it",
        href: DOC("running.md"),
      },
    ],
  },

  tech: {
    eyebrow: "Under The Hood",
    title: "Playing in sync is hard. ondaire does the hard part.",
    intro:
      "Getting two speakers to play the very same instant over flaky Wi-Fi means fighting both the network and physics at once — and then proving you actually won. Below are the four problems that pull rooms apart and the fix for each, followed by the measurements that back it up: first from a microphone in the room, then from the cluster’s own live telemetry. Four problems, four fixes:",
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
      "Anyone can claim “perfect sync,” so we did the harder thing and measured it from the air. A single microphone in the room with two Raspberry-Pi speakers, a calibrated sine sweep, and a matched filter recover each speaker’s acoustic arrival time to a fraction of a sample — that’s the real gap your ears would hear, room reflections and all. Ten minutes, every burst, nothing smoothed away. Two views of the same recording:",
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

  // Clock telemetry proof. No microphone — the cluster's own per-node numbers,
  // polled once a second for 20 minutes by tools/calib/clock_telemetry.py. Graphs
  // are bare PNGs (clock_drift / clock_correction / clock_silence); headline +
  // explanation live here. Numbers are from the real zero-01 / zero-02 capture.
  clocks: {
    eyebrow: "Caught in the act",
    title: "The same story, in the cluster’s own numbers.",
    intro:
      "No microphone this time — just the cluster’s own telemetry. We played for twenty minutes and polled every node once a second: each one reports its clock offset to the master and exactly how many samples the playout servo injected, dropped, or replaced with silence. Two real Raspberry-Pi Zero players (zero-01, zero-02) and a master (study) as the time reference. Graphed raw.",
    items: [
      {
        src: "assets/img/clock_drift.png",
        alt: "Clock drift rate over 20 minutes: the master is a flat zero line; zero-01 sits near +12 ppm and zero-02 near +15 ppm, each a roughly flat line wandering a little.",
        kicker: "The problem · crystal drift",
        metric: "+12 & +15 ppm",
        title: "Three clocks, three speeds",
        body:
          "The master clock is the straight zero line. Each Pi’s quartz runs at its own measured rate — zero-01 about +12 ppm fast, zero-02 about +15 ppm — and this is the rate, not an accumulating offset: a roughly flat line that only wanders as the boards warm. Left uncorrected, +15 ppm pulls a speaker about a millisecond out of sync every seventy seconds, and seconds apart over an evening. This is the raw measurement — the slope of each device’s reported clock offset — not a spec-sheet figure.",
      },
      {
        src: "assets/img/clock_correction.png",
        alt: "Injection and drop rate per node: injected hovers on each crystal’s ppm line, dropped stays near zero.",
        kicker: "The fix · rate matching",
        metric: "net ≈ 0 ppm",
        title: "Cancelled, sample by sample",
        body:
          "The playout servo resamples each node by a hair to erase that drift: it injects a duplicate sample now and then — at about +13 and +14 ppm, landing right on each crystal’s own drift line (faint) — and drops almost nothing. Injected minus dropped nets to essentially zero against the master, which is why the rooms stay phase-locked. The independent clock measurement, the servo’s own reported rate, and this realized injection rate all agree to within about a part per million — three different signals telling the same story.",
      },
      {
        src: "assets/img/clock_silence.png",
        alt: "Silence inserted per minute: a low trickle under ~1 ms/min for both nodes, totalling 13 ms and 4 ms over the run.",
        kicker: "The cost · dropouts",
        metric: "13 ms in 20 min",
        title: "Almost no silence at all",
        body:
          "When a buffer briefly runs dry the node outputs silence rather than the wrong sample. Across the full twenty minutes that came to 13 ms on zero-01 and 4 ms on zero-02 — a steady trickle well under a millisecond per minute, roughly 0.001% of the session, none of it clustered into an audible gap. Honest read: this is a real Wi-Fi link, so it isn’t zero; it’s just small enough not to hear.",
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
        // `credit` renders a license pill linking to the source under the photo.
        // Pre-1900 portrait paintings are public domain regardless of the scan.
        // The 20th-century PHOTOS below need each vendored file's exact Commons
        // source + license filled in (there are many differently-licensed files
        // per artist, so they can't be safely guessed).
        credit: { license: "Public domain", href: "https://commons.wikimedia.org/wiki/Category:Ludwig_van_Beethoven" },
      },
      {
        name: "Wolfgang Amadeus Mozart",
        role: "Composer · 1756–1791",
        quote:
          "Setup took less time than a cadenza. One file, no fuss — even a prodigy could manage it.",
        img: "assets/img/testimonials/mozart.jpg",
        credit: { license: "Public domain", href: "https://commons.wikimedia.org/wiki/Category:Wolfgang_Amadeus_Mozart" },
      },
      {
        name: "Johann Sebastian Bach",
        role: "Composer · 1685–1750",
        quote:
          "Every voice entering at precisely the right instant, in every room at once. Counterpoint, but for speakers.",
        img: "assets/img/testimonials/bach.jpg",
        credit: { license: "Public domain", href: "https://commons.wikimedia.org/wiki/Category:Johann_Sebastian_Bach" },
      },
      {
        name: "Miles Davis",
        role: "Jazz · 1926–1991",
        quote:
          "It’s not the notes you sync, it’s the ones you don’t. ondaire gets the silence right in all five rooms.",
        img: "assets/img/testimonials/miles-davis.jpg",
        credit: { author: "Tom Palumbo", license: "CC BY-SA 2.0", href: "https://commons.wikimedia.org/wiki/File:Miles_Davis_by_Palumbo.jpg" },
      },
      {
        name: "Ella Fitzgerald",
        role: "Jazz vocalist · 1917–1996",
        quote:
          "No cloud, no accounts, no scat about subscriptions. Put it on and the whole house swings.",
        img: "assets/img/testimonials/ella-fitzgerald.jpg",
        credit: { author: "William P. Gottlieb", license: "Public domain", href: "https://commons.wikimedia.org/wiki/File:Ella_Fitzgerald_(Gottlieb_02871).jpg" },
      },
      {
        name: "Freddie Mercury",
        role: "Rock · 1946–1991",
        quote:
          "I want it all, I want it all — and I want it in every room. Darling, it delivered.",
        img: "assets/img/testimonials/freddie-mercury.jpg",
        credit: { author: "Carl Lender", license: "CC BY-SA 3.0", href: "https://commons.wikimedia.org/wiki/File:Freddie_Mercury_performing_in_New_Haven,_CT,_November_1977_cropped.jpg" },
      },
      {
        name: "Jimi Hendrix",
        role: "Rock guitarist · 1942–1970",
        quote:
          "’Scuse me while I sync the sky. Kitchen, hallway, garage — all phase-locked. Far out.",
        img: "assets/img/testimonials/jimi-hendrix.jpg",
        credit: { author: "A. Vente", license: "CC BY-SA 3.0 NL", href: "https://commons.wikimedia.org/wiki/File:Jimi_Hendrix_(1967).jpg" },
      },
      {
        name: "Prince",
        role: "Pop · 1958–2016",
        quote:
          "Dearly beloved, we are gathered to play one song in every room. No latency, no cloud. Just the music.",
        img: "assets/img/testimonials/prince.jpg",
        credit: { author: "Scott Penner", license: "CC BY-SA 2.0", href: "https://commons.wikimedia.org/wiki/File:Prince_at_Coachella.jpg" },
      },
    ],
  },

  // Honest authorship note — AI-assisted, human-understood. Same wry, candid
  // voice as the proof section. Rendered as a centered colophon before the CTA.
  authorship: {
    eyebrow: "Made by a human",
    title: "AI helped write it. A person understands all of it.",
    body: [
      "Yes — I used Claude for much of this code. No, it isn’t vibe-coded AI slop. The architecture, the clock-sync math, the wire protocol, the trade-offs behind every fix above: that’s my thinking and my experience, and I know and understand pretty much every line that ships.",
      "I used AI the way you’d use any good power tool — to get past my own laziness and move faster, not to outsource the judgement. If something in here is wrong, that’s on me, not on a model.",
    ],
  },

  cta: {
    title: "Bring your speakers together.",
    body:
      "Grab a build for your device, or read the user guide to see the whole app first.",
    primary: { label: "Get it", href: "download.html" },
    secondary: { label: "Read the guide", href: GUIDE },
  },

  // The dedicated Home Assistant integration page (home-assistant.html):
  // screenshots (desktop + mobile), what it does, and the install steps with a
  // restart callout. Linked from the download card and the install page.
  homeAssistant: {
    eyebrow: "Home Assistant",
    title: "Every room, right in Home Assistant.",
    intro:
      "The ondaire integration turns each room into a Home Assistant media_player — play/pause, volume, and speaker grouping — and ships a custom Lovelace card that mirrors the ondaire app: players, the media library, stream presets, and the shared queue. Every request is proxied through Home Assistant, so your dashboard talks only to HA.",
    shots: [
      {
        src: "assets/img/home_assistant.png",
        alt: "Ondaire room cards on a Home Assistant dashboard",
        cap: "One card per room on a desktop dashboard.",
        wide: true, // landscape shot — use the roomier lightbox slide
      },
      {
        src: "assets/img/home_assistant_mobile.png",
        alt: "An ondaire room card in the Home Assistant companion app",
        cap: "The same card in the companion app.",
      },
    ],
    features: [
      "Each room is a media_player: transport, group volume, and join/unjoin.",
      "A tabbed Lovelace card — Players, Media, Streams, Queue.",
      "Toggle any speaker into a room; per-speaker and proportional group volume.",
      "Browse and search your library, queue tracks or whole folders, play stream presets.",
    ],
    steps: [
      "Download the integration zip and unzip it into your Home Assistant <code>config</code> folder so the files land in <code>&lt;config&gt;/custom_components/ondaire/</code>.",
      "Restart Home Assistant.",
      "Add a room under <em>Settings → Devices &amp; services</em> — ondaire masters are auto-discovered over mDNS, or enter a host and port.",
      "Hard-refresh your browser once, then add an <code>ondaire-card</code> to a dashboard (one per room).",
    ],
    important:
      "A full <strong>restart</strong> of Home Assistant is required after copying the files — a reload of the integration alone won't pick up a brand-new component. When you <em>update</em> the integration later, restart again and hard-refresh the browser (in the companion app, pull down to refresh or clear the app's cache) so the new card loads.",
    download: { label: "Download the integration", href: "download.html" },
    readme: { label: "Full README", href: `${REPO}/-/tree/main/integrations/homeassistant` },
  },

  // The separate download page (download.html). Each option's `file` is resolved
  // at build time: build.mjs hashes the staged binary (src/assets/downloads/) and
  // fills in its SHA-256 + size. `docker` options carry a pull command instead.
  download: {
    eyebrow: "Download",
    title: "Get ondaire for your hardware.",
    intro:
      "One small, static binary per device — pure Go, no runtime, no dependencies. Each archive is the build attached to the matching tagged release: verify its SHA-256, unpack it, and run ./ondaire. Prefer containers? Pull the master image with Spotify Connect built in.",
    // Prominent callout rendered above the intro: Wi-Fi contention is the #1 field issue.
    important:
      "The number of devices on your Wi-Fi alone does matter — not just activity, throughput and signal strength. For best results, have a separate Wi-Fi (on a different channel) for ondaire, with a single SSID — for ESP32, make it 2.4GHz only.",
    // Caveat rendered as a tip under the page intro: a uniform fleet syncs best.
    note:
      "For the tightest sync, use the same TYPE of player throughout — e.g. all Raspberry Pi nodes, or all ESP32 nodes. Mixed fleets work (the master equalizes each node's output latency), but identical hardware shares the same latency and clock behaviour, so it lines up best.",
    // Teaser + entry point to the browser flasher (flash.html).
    // See docs/developer/esp32.md + docs/developer/player-protocol.md.
    esp32: {
      badge: "DIY",
      title: "ESP32 players",
      body:
        "Turn a PSRAM-equipped ESP32 + an I2S DAC into a real ondaire player: it shows up in the cluster, joins any group, and plays in lock-step like every other room — flashed straight from your browser, no toolchain. Tested on the ESP32-S3 Super Mini and the Waveshare ESP32-S3-Zero (both PSRAM) with a PCM5102A DAC.",
      href: "flash.html",
      hrefLabel: "Open the browser flasher",
    },
    // Guided installer (scripts/get.sh) — the fastest path onto a Linux box.
    // Rendered after the ESP32 teaser, before the per-arch download cards.
    installer: {
      title: "Installer",
      body:
        "The quickest way onto a Linux box: a guided one-liner. It detects your OS and CPU, downloads the matching ondaire build, then walks you through the optional extras — Spotify Connect (go-librespot) and a boot-time systemd service — and sets them up. Installs into /usr/local/lib/ondaire; re-run any time to update.",
      code: "curl -fsSL https://ondaire.rand0m.me/get.sh | sudo bash",
      // Expandable, annotated walkthrough of scripts/get.sh — its real flow with
      // section labels + explanatory comments (not the verbatim 256 lines).
      walkthrough: {
        summary: "What the script does, step by step",
        hrefLabel: "Read the full get.sh",
        href: `${REPO}/-/blob/main/scripts/get.sh`,
        script: `#!/usr/bin/env bash
#   curl -fsSL https://ondaire.rand0m.me/get.sh | sudo bash
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
# Binary lands in /usr/local/lib/ondaire, symlinked onto your PATH.
fetch "$BASE/assets/downloads/ondaire-linux-$ARCH.tar.gz" | tar -xz
install -m755 ondaire /usr/local/lib/ondaire/ondaire
ln -sf  /usr/local/lib/ondaire/ondaire /usr/local/bin/ondaire

# ── 4. Choose a role (prompts read the terminal, so they work
#       even under  curl | bash) ─────────────────────────────────
if ask "Run the web UI on this node?"; then
  ROLE="master,playback"      # serves the UI, gossips, can play audio
else
  ROLE="playback"             # receive-only, driven by a master
fi

# ── 5. Spotify Connect — masters only, optional ────────────────
# Fetches the matching go-librespot build alongside ondaire.
[ "$ROLE" != playback ] && ask "Install Spotify Connect?" && install_go_librespot

# ── 6. Boot service — optional ─────────────────────────────────
# Writes /etc/systemd/system/ondaire.service and enables it, so
# ondaire starts at boot and restarts on failure.
ask "Start ondaire at boot (systemd)?" && install_systemd_unit

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
        file: "assets/downloads/ondaire-linux-arm64.tar.gz",
        note:
          "Heads-up: Opus playback loads <strong>libopus</strong> at runtime. The Desktop image already has it; on a minimal Lite install that lacks it, add it with <code>sudo apt install libopus0</code> (uncompressed PCM works without it). Audio hardware that needs third-party drivers is out of ondaire's scope — get the card working in Linux first.",
      },
      {
        name: "PC or server — x86-64",
        rec: "Any modern Linux distribution on a 64-bit Intel/AMD machine.",
        arch: "linux · amd64",
        logos: ["fedora", "ubuntu", "debian", "arch", "manjaro"],
        file: "assets/downloads/ondaire-linux-amd64.tar.gz",
      },
      {
        name: "Docker",
        rec: "For a NAS or server: runs a master — mount your music library and serve it to your players, with Spotify Connect (go-librespot) built in. Multi-arch: amd64 · arm64.",
        arch: "container image",
        logos: ["docker"],
        docker: [
          "docker run -d --network host \\",
          "  -v /srv/music:/media:ro \\",
          "  -v ondaire-data:/data \\",
          "  harbor.rand0m.me/public/ondaire:latest --name living-room",
        ].join("\n"),
        // First paragraph: trusted author HTML, rendered as plain body text.
        body:
          "The image runs the <strong>master role only</strong> by default — it sources audio (your library + Spotify Connect) and controls the cluster, but does not play locally. Reaching host audio hardware (sound cards, USB DACs, I2S) from a container isn't reliable and is <strong>out of scope</strong>: the actual output happens on your players (the Pi binary, or an ESP32 node). Mount your library read-only on <code>/media</code>; mutable state lives on <code>/data</code>. You can change any default with the <code>ONDAIRE_*</code> environment variables — see the config flags below.",
        // The one genuine gotcha, kept as a callout.
        note:
          "<strong><code>--network host</code> is required.</strong> Players discover the master over mDNS and go-librespot advertises Spotify Connect over zeroconf — multicast doesn't cross Docker's bridge network (and ports bind-or-increment, so there's nothing to publish).",
      },
      {
        name: "Home Assistant",
        rec: "Control every room from Home Assistant: each room becomes a media_player, plus a custom Lovelace card with tabs for players, media, streams and the queue.",
        arch: "custom integration",
        logos: ["homeassistant"],
        file: "assets/downloads/ondaire-hacs.zip",
        note:
          "Unzip into <code>&lt;config&gt;/custom_components/ondaire/</code> and restart Home Assistant. " +
          '<a href="home-assistant.html">Setup, screenshots &amp; details →</a>',
      },
    ],
    // Common config flags — rendered as its own block UNDER the download cards.
    flags: {
      title: "Common config flags",
      intro:
        "Almost every flag has an ONDAIRE_* environment equivalent (handy in Docker); all are optional, with sensible defaults. Ports left at their default bind-or-increment (run several nodes on one box); a port you set explicitly is pinned (binds exactly or exits).",
      // Rendered as a param · env var · default · description table.
      cols: ["param", "env var", "default", "description"],
      params: [
        { param: "--name <name>", env: "—", def: "node id", what: "display name + Spotify device name (first start only)" },
        { param: "--role <role>", env: "ONDAIRE_ROLE", def: "both", what: "master | playback | master,playback" },
        { param: "--media <dir>", env: "ONDAIRE_MEDIA_DIR", def: "<data>/media", what: "library directory, browsed recursively" },
        { param: "--data <dir>", env: "ONDAIRE_DATA_DIR", def: "./data", what: "node.json, cluster state, Spotify creds" },
        { param: "--http-port <n>", env: "ONDAIRE_HTTP_PORT", def: "8080", what: "UI + REST API + WebSocket + node proxy" },
        { param: "--output <spec>", env: "ONDAIRE_OUTPUT", def: "auto", what: "alsa · exec · null · file:<path>" },
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
  // and flashes the matching image from `firmware.builds`. Two flash modes (a
  // radio on the install step): "flash everything" writes the merged image and
  // wipes config, so the node reboots into its Wi-Fi captive portal for setup;
  // "update firmware only" rewrites just the app partition and keeps stored
  // Wi-Fi/name/pins. There is no in-browser provisioning — the captive portal
  // (or the USB JSON console) handles that. Three-step wizard, one panel at a
  // time: (1) select board → (2) install → (3) finished.
  flash: {
    eyebrow: "Build a node",
    title: "Flash a DIY speaker, right from your browser.",
    intro:
      "Turn an ESP32 + an I2S DAC (or an all-in-one amp board) into a real ondaire player — it shows up in the cluster, joins any group, and plays in lock-step like every other room. No toolchain, no app: plug it in over USB-C in Chrome or Edge and flash. On first boot it hosts a Wi-Fi hotspot for setup. Receive-only, opus over Wi-Fi.",
    // Progress header — one chip per wizard step.
    wizard: [
      { id: "board", label: "Select board" },
      { id: "install", label: "Install" },
      { id: "finished", label: "Finished" },
    ],
    // Step 1 — board picker. Nothing downstream unlocks until a board is chosen.
    board: {
      title: "Select your board",
      body: "Pick the board you’re flashing — the firmware bakes in that board’s pins and DAC wiring, so the only thing left to set later is your Wi-Fi.",
      next: "Connect",
    },
    bom: {
      title: "What you need",
      items: [
        "A PSRAM-equipped ESP32 board — an ESP32-S3 (DevKitC-1 or Waveshare S3-Zero) or a classic ESP32-WROVER.",
        "A PCM5102A I2S DAC (the common purple GY-PCM5102 module).",
        "A KY-040 / EC11 rotary encoder for local volume (optional).",
        "A USB-C cable and Chrome or Edge on desktop.",
      ],
      // The all-in-one alternative to the whole DIY list above — rendered as a
      // single bullet under an "or" divider, same font as the list; only the
      // linked "Sonocotta ↗" is bold.
      alt: {
        html: 'An all-in-one <a href="https://sonocotta.com" target="_blank" rel="noopener" class="fl-bom-alt-link"><strong>Sonocotta</strong> <span class="fl-ext" aria-hidden="true">↗</span></a> board — DAC and amp already on board, so there’s no separate DAC to wire.',
      },
    },
    // Step 2 — install. ESP Web Tools detects the chip and flashes; a radio picks
    // the mode → which manifest the installer uses (flash-all merged @ 0x0 vs
    // keep-config app @ 0x20000). No fields here — provisioning is the captive
    // portal after a full flash.
    install: {
      title: "Install the firmware",
      requirements:
        "Plug the board in over USB-C. First flash on an S2/S3 may need download mode: hold BOOT, tap RESET, release BOOT — then click Flash. Needs Chrome or Edge on desktop (Web Serial).",
      modes: {
        title: "What to flash",
        all: {
          label: "Flash everything (first-time setup)",
          note: "Writes a clean image and clears any stored settings. On reboot the node hosts a Wi-Fi hotspot named “ondaire-…”: join it from a phone or laptop and a setup page opens where you enter your 2.4 GHz Wi-Fi. Pick this for a new board or a clean slate.",
        },
        keep: {
          label: "Update firmware only (keep my config)",
          note: "Rewrites just the app and leaves the node’s stored Wi-Fi, name, and pin config untouched. Use this to upgrade a node you’ve already set up — it reboots and rejoins your network on its own.",
        },
      },
      action: "Flash",
      next: "Finish",
      okMsg: "Flashed successfully — continue.",
      errMsg:
        "Flashing didn’t finish. Check the USB-C cable, try download mode (hold BOOT, tap RESET, release BOOT), then flash again.",
      unknownMsg: "When the installer reports it’s done, continue.",
    },
    // Step 3 — finished. Congratulations + how to finish setup (captive portal for
    // a full flash) + a link to the board's page (wiring, pinouts, build notes).
    finished: {
      title: "You’ve flashed an ondaire node.",
      body: "If you flashed everything, the node is now hosting a Wi-Fi hotspot named “ondaire-…”. Join it from your phone or laptop and a setup page opens — enter your 2.4 GHz Wi-Fi and the node reboots, joins your network, and appears in the cluster within a few seconds, ready to join any group. (Updated firmware only? It rejoins on its own.)",
      warning:
        "ESP32 joins 2.4 GHz Wi-Fi only. If your router uses band steering (one shared name for the 2.4 and 5 GHz bands), give the 2.4 GHz band its own SSID if you can — otherwise the node may keep trying the 5 GHz radio and fail to connect.",
      docLink: "Wiring & board guide",
    },
    docHref: `${REPO}/-/blob/main/docs/developer/esp32.md`,
    docLabel: "Firmware & hardware guide",
  },

  // Boards offered by the flasher, one entry per flashable board. Each is its own
  // dropdown option; selecting it reveals the board photo + the matching ESP Web
  // Tools manifest. `file` is resolved at build time (build.mjs resolveFirmware)
  // into a per-board manifest (manifest-<id>.json) + SHA-256. `id` must match the
  // esp32 board profile (esp32/boards/ + sdkconfig.defaults.<id>) so the locally
  // built merged image (esp32/build-<id>/ondaire-fw-<id>.bin) lines up.
  firmware: {
    manifestName: "ondaire player",
    // Each build maps to a CI firmware image (ondaire-fw-<id>.bin + the app-only
    // ondaire-app-<id>.bin) and its own ESP Web Tools manifest pair. `tested: true`
    // means the image has been flashed + verified on real hardware; untested boards
    // are built pin-map-correct from the vendor's schematics/ESPHome configs but not
    // yet bench-checked — the flasher badges them so people know (all pins are
    // re-provisionable over USB if a default is off). The Sonocotta family shares one
    // "audio dock" S3 carrier; the Mini/Pro use the same firmware as the Louder/
    // Louder-Plus (same DAC + pin map), so they flash those images.
    builds: [
      {
        id: "esp32s3-supermini",
        chipFamily: "ESP32-S3",
        label: "ESP32-S3 Super Mini (PSRAM version)",
        note: "Dual-core ESP32-S3 + 2 MB PSRAM, native USB-C. Pair with a PCM5102A I2S DAC.",
        tested: true,
        // Board photo (front + back) — canonical copy lives next to the board's
        // sheet in esp32/devices/; build.mjs copies it into the site like `wiring`.
        // Marketing board photo (front + back). Lives in site/src/assets/img/ — the
        // Docker build context is site/ only, so it ships via copyDir (the same
        // photo also sits in esp32/devices/ for the GitLab device sheet).
        img: "assets/img/esp32-s3-super-mini.jpg",
        // The board's own page in the repo (wiring, pinouts, build notes). Since the
        // flasher knows the selected board, the finished step links straight to it.
        doc: `${REPO}/-/blob/main/esp32/devices/esp32-s3-super-mini.md`,
        file: "assets/firmware/ondaire-fw-esp32s3-supermini.bin",
      },
      {
        id: "esp32s3-zero",
        chipFamily: "ESP32-S3",
        label: "Waveshare ESP32-S3-Zero",
        note: "Dual-core ESP32-S3 + 2 MB PSRAM, native USB-C, 23.5 × 18 mm. Pair with a PCM5102A I2S DAC.",
        tested: true,
        // Marketing board photo. Lives in site/src/assets/img/ — the Docker build
        // context is site/ only, so it ships via copyDir (the same photo also sits
        // in esp32/devices/ for the GitLab device sheet).
        img: "assets/img/esp32-s3-zero.jpg",
        doc: `${REPO}/-/blob/main/esp32/devices/esp32-s3-zero.md`,
        file: "assets/firmware/ondaire-fw-esp32s3-zero.bin",
      },
      {
        id: "esp32s3-hifi",
        chipFamily: "ESP32-S3",
        label: "Sonocotta HiFi-ESP32-S3",
        note: "ESP32-S3 audio dock with a PCM5100A I2S DAC — line-out only (no amp). 8 MB PSRAM.",
        tested: false,
        img: "assets/img/hifi-esp32.jpg",
        doc: `${REPO}/-/blob/main/esp32/devices/sonocotta-audio-dock.md#hifi-esp32-s3`,
        file: "assets/firmware/ondaire-fw-esp32s3-hifi.bin",
      },
      {
        id: "esp32s3-hifi-plus",
        chipFamily: "ESP32-S3",
        label: "Sonocotta HiFi-ESP32-Plus",
        note: "ESP32-S3 audio dock with a PCM5122 DSP DAC — line-out only (no amp). 8 MB PSRAM.",
        tested: false,
        img: "assets/img/hifi-esp32-plus.jpg",
        doc: `${REPO}/-/blob/main/esp32/devices/sonocotta-audio-dock.md#hifi-esp32-plus`,
        file: "assets/firmware/ondaire-fw-esp32s3-hifi-plus.bin",
      },
      {
        id: "esp32s3-amped",
        chipFamily: "ESP32-S3",
        label: "Sonocotta Amped-ESP32-S3",
        note: "All-in-one: ESP32-S3 + PCM5100A DAC + TPA3110 amp (2×25 W). Drives passive speakers directly. 8 MB PSRAM.",
        tested: false,
        img: "assets/img/amped-esp32.jpg",
        doc: `${REPO}/-/blob/main/esp32/devices/sonocotta-audio-dock.md#amped-esp32-s3`,
        file: "assets/firmware/ondaire-fw-esp32s3-amped.bin",
      },
      {
        id: "esp32s3-amped-plus",
        chipFamily: "ESP32-S3",
        label: "Sonocotta Amped-ESP32-S3-Plus",
        note: "All-in-one: ESP32-S3 + PCM5122 DAC + TPA3110 amp on one board (8 MB PSRAM). Drives passive speakers directly — no separate DAC to wire.",
        tested: true,
        // Board photo (site copy — the Docker build context is site/ only). Lives
        // in site/src/assets/img/; ships via copyDir like the other boards' photos.
        img: "assets/img/amped-esp32-s3-plus.jpg",
        doc: `${REPO}/-/blob/main/esp32/devices/amped-esp32-s3-plus.md`,
        file: "assets/firmware/ondaire-fw-esp32s3-amped-plus.bin",
      },
      {
        id: "esp32s3-loud",
        chipFamily: "ESP32-S3",
        label: "Sonocotta Loud-ESP32-S3",
        note: "All-in-one: ESP32-S3 + dual MAX98357A I2S DAC+amp (2×5 W). Drives passive speakers directly. 8 MB PSRAM.",
        tested: false,
        img: "assets/img/loud-esp32.jpg",
        doc: `${REPO}/-/blob/main/esp32/devices/sonocotta-audio-dock.md#loud-esp32-s3`,
        file: "assets/firmware/ondaire-fw-esp32s3-loud.bin",
      },
      {
        id: "esp32s3-loud-plus",
        chipFamily: "ESP32-S3",
        label: "Sonocotta Loud-ESP32-Plus",
        note: "All-in-one: ESP32-S3 + Infineon MA12070P DSP amp (2×60 W). Drives passive speakers directly. 8 MB PSRAM.",
        tested: false,
        img: "assets/img/loud-esp32-plus.jpg",
        doc: `${REPO}/-/blob/main/esp32/devices/sonocotta-audio-dock.md#loud-esp32-plus`,
        file: "assets/firmware/ondaire-fw-esp32s3-loud-plus.bin",
      },
      {
        id: "esp32s3-louder",
        chipFamily: "ESP32-S3",
        label: "Sonocotta Louder-ESP32-S3 / Mini",
        note: "All-in-one: ESP32-S3 + TAS5805M DSP amp (2×32 W into 8Ω). Same image fits the Louder-ESP32-Mini. 8 MB PSRAM.",
        tested: false,
        img: "assets/img/louder-esp32.jpg",
        doc: `${REPO}/-/blob/main/esp32/devices/sonocotta-audio-dock.md#louder-esp32-s3-mini`,
        file: "assets/firmware/ondaire-fw-esp32s3-louder.bin",
      },
      {
        id: "esp32s3-louder-plus",
        chipFamily: "ESP32-S3",
        label: "Sonocotta Louder-ESP32-Plus / Pro",
        note: "All-in-one: ESP32-S3 + TAS5825M DSP amp (2×30 W). Same image fits the Louder-ESP32-Pro. 8 MB PSRAM.",
        tested: false,
        img: "assets/img/louder-esp32-plus.jpg",
        doc: `${REPO}/-/blob/main/esp32/devices/sonocotta-audio-dock.md#louder-esp32-plus-pro`,
        file: "assets/firmware/ondaire-fw-esp32s3-louder-plus.bin",
      },
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
