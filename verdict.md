# ondaire — a skeptic's read

*Notes from an experienced developer / smart-home tinkerer who found the
marketing site, read it with a raised eyebrow, then went and read the source.*

## First impression (the site)

The landing copy is *suspiciously* clean. "Every room. One sound. Zero fuss."
→ `./ondaire` → "That's the whole setup." My reflex with that kind of pitch is
to assume it's hiding a config nightmare behind a hero animation. Snapcast does
multiroom too, and anyone who's actually run Snapcast knows the gap between "it
syncs!" and "it syncs reliably on my WiFi" is where the weekends go.

**What I liked immediately:**

- It names the hard problems instead of papering over them. The "Under the hood"
  section literally lists *network jitter, packet loss, system clock drift, DAC
  clock drift* and gives a one-paragraph fix for each. That's not marketing-brain
  — that's someone who's been bitten by all four. The DAC-drift one especially
  ("no two sound cards sample at exactly 48 kHz… rooms slide out of phase over a
  long track") is the kind of thing you only write if you've watched two speakers
  go out of phase 40 minutes into an album. Most products never even admit that's
  a thing.
- **No cloud, no accounts, no telemetry** is front and center and — see below —
  actually true in the code. I'm allergic to smart-home gear that phones home.
  This doesn't.
- The honesty in the README's *"Status & scope"*: "Out of scope for now:
  auth/TLS, internet-facing operation, playlists… v1 is a trusted-LAN system."
  Vendors don't usually tell you what their thing *can't* do. This earns a lot of
  trust.

**What made me squint:**

- "No config" / "first run just works" is doing heavy lifting. It rests entirely
  on **mDNS discovery**, and mDNS is exactly the thing that falls over on real
  home networks — multiple APs, a mesh system, IoT VLAN isolation, `igmp
  snooping` on a cheap switch. The site says "peers appear within seconds." On a
  flat LAN, sure. On my segmented setup with the IoT devices walled off? I'd bet
  money I'm reaching for `--no-mdns --join`. To their credit that escape hatch
  *exists* and is documented — but the marketing tone undersells how often you'll
  need it.
- It's hosted on a personal GitLab (`gitlab.rand0m.me`), not GitHub. No stars, no
  issue tracker I can size up, no "who else runs this." For a thing I'm putting on
  every speaker in my house, "one person's project" is a real risk factor.
  Bus-factor of one.
- **Linux-only audio.** Pure-Go binary, great, but the sink is ALSA or shelling
  out to `pw-play`/`aplay`. No native macOS/Windows output. Fine for my Pis, but
  "your whole home" quietly means "your whole home, as long as every endpoint is
  a Linux box."
- Spotify Connect "built in via go-librespot." That's a reverse-engineered client
  riding Spotify's ToS goodwill. Nice that it's there; I wouldn't build my setup's
  reliability on it.

## Then I read the code (this is where it won me over)

I half-expected the substance to not match the pitch. It does. ~34k lines of Go,
a test file next to basically every source file, clean `internal/` package
boundaries (`clock`, `stream`, `source`, `sink`, `cluster`, `group`…).

The things I specifically went to verify:

- **"Pure Go, no cgo" — true.** No `import "C"` anywhere. ALSA and Opus are
  pulled in at *runtime* via `dlopen` (purego, in `internal/dl`). That's the
  right call: one binary, `amd64`/`arm64`/Pi Zero, degrades to a null sink if the
  libs aren't present. This is the claim I was most ready to catch them cheating
  on, and they didn't.

- **The rate servo is real engineering, not a buzzword.** `internal/sink/servo.go`
  holds the device queue at a setpoint with a gentle proportional controller, and
  the *comments document a past failure*: `Kq=1.5` mapped the Pi's ±10ms
  `snd_pcm_delay` jitter to >300 ppm and "railed the loop into hunting that was
  the audible stumbling." They dialed it to `0.08`. You don't write that comment
  unless you actually shipped the bug, heard it, and fixed it. That single comment
  did more to convince me than the entire website.

- **The architecture is honestly the elegant part.** The only replicated fact in
  the cluster is each node's `following` field — group membership, group ID, and
  failover all *fall out* of that, recomputed everywhere from gossiped state
  (HashiCorp memberlist). That's a genuinely tasteful design. Less state to
  corrupt, less to disagree about.

- **The "why" is written down.** The architecture docs don't just describe the
  system, they argue for it — why a *proportional* servo and not PI, why the group ID
  is the master's node ID, why opus is the default with a pcm fallback — and the code
  back-references the choices it implements. This is the discipline of someone who's
  maintained long-lived systems. I'd be comfortable picking this up cold in a year.

**Where my skepticism survived contact with the code:**

- **The FEC is weak by design.** It's plain XOR parity over blocks of 4 frames —
  recovers *exactly one* lost frame per block. A WiFi burst loss (which is the
  *normal* failure mode — losses clump, they don't arrive politely spaced out) of
  2+ frames in a window blows right through it. The fallback is "flip to TCP,"
  which trades latency for certainty. So the real story is "FEC handles light
  loss; lean on TCP when your WiFi is bad" — which is fine, but it's less magic
  than the site implies.
- **Trusted-LAN, full stop.** "No auth (trusted LAN, v1)." Anyone on your network
  can drive any node's API and blast audio. For me that means it lives on the IoT
  VLAN and never touches the internet. Acceptable for v1, but it's a hard ceiling
  on where you can deploy it.
- Sync precision claims: "aligned to the millisecond." The mechanism
  (wall-anchored monotonic clock, per-player offset/RTT measurement, playout
  deadlines) is sound and is the same approach the good systems use. I'd want to
  actually *hear* two speakers a meter apart before I believed the number — but
  nothing in the design makes it implausible.

## Verdict

**Would I use it?** Yes — for exactly what it says it is. On my LAN, on Pis I
already own, with music I already have, behind a VLAN, with realistic
expectations about mDNS and a willingness to fall back to `--join` and TCP. The
thing I care most about in a hobby audio setup — *no cloud, runs on hardware I
control, one binary, source I can read* — it nails. And the code quality means
that when something breaks at 11pm, I can actually go find out why.

**Would I recommend it?** With caveats, to the right person — i.e. someone like
me. To a fellow tinkerer who's comfortable on a Linux box and reading a flag
table: enthusiastically. To a normal person who wants "Sonos but free": no — the
trusted-LAN security model, the Linux-only endpoints, the single-maintainer bus
factor, and the mDNS-or-bust first-run all need a babysitter who knows what
they're doing.

**The one-line summary I'd actually text a friend:** *"Found a multiroom audio
thing — single Go binary, no cloud, no accounts, code is genuinely good and the
author is honest about what it can't do. It's Snapcast-adjacent but the design's
cleaner. LAN-only and Linux-only though. I'm trying it on the Pis this weekend."*

The rare case where reading the source made me trust the marketing **more**, not
less. That basically never happens.
