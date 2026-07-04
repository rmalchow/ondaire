# Developer docs

Specs and plans for building on ondaire: implementing players and firmware, the
hardware nodes, measurement tooling, and in-flight engineering work. For the system
design itself, see [architecture](../architecture/).

| Doc | What it is |
|---|---|
| [Player protocol](player-protocol.md) | The **self-contained** byte-level spec for a receive-only player (a Go playback daemon or ESP32 firmware). You can implement a conforming player from this document alone. |
| [ESP32 node](esp32.md) | Firmware + hardware design for the ESP32 speaker node: reference board matrix, pin maps, NVS config, the web flasher, OTA, and the conformance bar. |
| [Calibration](calibration.md) | Measuring real at-the-speaker acoustic coherence, building it into the master as an on-demand calibration, and surfacing per-node sync health. |
| [Roadmap: playback nodes](roadmap-playback-nodes.md) | The phased plan for splitting a node into room (`master`) and player (`playback`) roles. Tracks what's landed vs. in progress. |

The standalone reference player is [`cmd/player`](../../cmd/player) (pure stdlib — it
exists to prove [player-protocol.md](player-protocol.md) is sufficient). The ESP32
firmware and flasher live under [`esp32/`](../../esp32) and
[`tools/calib/`](../../tools/calib) holds the calibration toolkit.
