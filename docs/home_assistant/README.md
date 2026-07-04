# Home Assistant integration — design & implementation plan

> Status: **plan only** (not yet implemented). This document is the agreed design for a
> Home Assistant custom integration that controls ondaire.

## Context

Ondaire is a Go daemon for multi-room synchronized audio. Every node runs one binary and
can be a **room** (`master` role — gossips, owns cluster state, serves the HTTP API, sources
audio, drives players) and/or a **player** (`playback` role — receive-only, no HTTP API of
its own, driven over UDP by its master; ESP32 speakers are playback-only). There is **no
existing external control surface beyond the REST + WebSocket HTTP API** — no Home Assistant,
MQTT, or `media_player` code anywhere in the repo. This is greenfield.

The goal is to let Home Assistant discover ondaire, browse the media library, and play out to
rooms/players. The idiomatic way is a **HA custom integration** (Python
`custom_components/ondaire`) that talks to a master node's HTTP API and mirrors live state
from its WebSocket feed. HA cannot run the Go daemon itself, so the integration is a *control
surface* over existing master node(s), not a new master.

**Agreed decisions:**
1. **One `media_player` per node** — rooms and players alike are speakers, each with its own
   volume; grouping via HA join/unjoin ↔ ondaire follow/unfollow (Sonos/HEOS convention).
2. **v1 = core** — playback + grouping + browse + push. Calibration/diagnostics deferred.
3. **Code lives in this repo** under `integrations/homeassistant/`, with a CI job to build a
   HACS-installable zip.

## Control surface (ground truth)

- Master HTTP API (Echo, JSON, **no auth**, trusted LAN) on `httpPort` (default **8080**).
- **Node proxy:** `/api/<nodeId-or-uniqueName>/<path>` reverse-proxies one hop to that node.
  So talking to ONE node reaches the whole cluster. Mutations targeting another node go through
  this. `base(nodeId)` = `/api` for self, else `/api/<nodeId>` (mirror the JS client).
- `GET /api/status` — connected node's own id/role (call once, to learn `self_id`).
- `GET /api/cluster` — full `Snapshot{nodes[], groups[], streamPresets[]}` (primary state).
- `GET /api/ws` — WebSocket, pushes `{type:"cluster", data:<Snapshot>}` on change (debounced
  250ms) + 5s heartbeat carrying `playback.positionSec`. **The push channel.**
- `GET /api/media` (per node) — flat `MediaFile{path,name,...}`, `/`-separated rel paths.
- `GET /api/cover?uri=file:<rel>` — cover bytes; `metadata.artUrl` is a direct URL (Spotify).
- Transport (master-only, 409 `not_master` else): `POST /api/play {uri}`, `/pause`, `/resume`,
  `/stop`, `/next`, `/seek {positionSec}` (409 `not_seekable` for live), `/queue {uris:[]}`.
- Per-node: `PATCH /api/node {volume(0..1), name, outputDelayMs, channel, ...}` (proxy-aware).
- Grouping: `POST /api/follow {target}` / `/unfollow` (normal nodes).
- **Playback-only nodes** (`playbackNode==true`, no HTTP API): mutate master-side via
  `POST /api/playback/patch {node, volume?, name?, following?, channel?}` — **never proxied**.
- mDNS `_ondaire._tcp.local.`; TXT always `id`,`role`,`ver`; masters add `name`,`http`.

**Model (crosswise):** every alive node always masters its own group; **group id == master
node id**. Groups are derived from each node's `following` field. "Join room X" = follow X.
`group.members[]` = all nodes following that master.

**Reference code to mirror (do not reinvent):**
- `web/src/lib/api.js` — proxy `base()`, the **playback-node-vs-normal branching**
  (`nodeSetVolume`, `assignToGroup`/`leaveGroup`), enqueue/seek/follow call shapes.
- `web/src/lib/ws.svelte.js` — WS reconnect/backoff (500→5000ms, 4s connect timeout) + node
  **roster failover** to another node's origin. Port this into the coordinator.
- `internal/contracts/contracts.go` — authoritative Snapshot/NodeView/GroupView/Playback/
  TrackMetadata JSON shapes to model in Python.
- `internal/api/api.go` — full route table. `internal/discovery/discovery.go` — TXT keys.

## Files to create

```
integrations/homeassistant/
  README.md                    # HACS + manual install, config, screenshots
  hacs.json                    # {"name":"ondaire","content_in_root":false}
  custom_components/ondaire/
    __init__.py                # async_setup_entry: build client+coordinator, forward platform
    manifest.json              # domain=ondaire, iot_class=local_push, integration_type=hub,
                               #   config_flow=true, zeroconf=["_ondaire._tcp.local."], version
    const.py                   # DOMAIN, PLATFORMS=[MEDIA_PLAYER], default port, WS path, backoff,
                               #   config keys (CONF_HOST, CONF_PORT, CONF_SELF_ID, CONF_ROSTER)
    models.py                  # frozen dataclasses mirroring contracts.go + from_json();
                               #   helpers: Snapshot.node(id), group_of(node_id), masters(),
                               #   smallest_master_id()
    api.py                     # OndaireClient(session, origin): base(node,self_id); get_status,
                               #   get_cluster, get_media, get_cover; play/pause/resume/stop/next/
                               #   seek/enqueue; patch_node; follow/unfollow; patch_playback;
                               #   node-aware set_volume/set_following (branch on playbackNode);
                               #   ws_connect(). Raises OndaireApiError(code,hint,status) on non-2xx
    coordinator.py             # OndaireCoordinator(DataUpdateCoordinator[Snapshot], push):
                               #   update_interval=None; owns WS task + backoff + roster failover;
                               #   parse frames → async_set_updated_data; capture position timestamp;
                               #   dispatch SIGNAL_ADD_ENTITIES for late-joining nodes
    config_flow.py             # zeroconf (masters only) + manual host; dedup via
                               #   async_set_unique_id(smallest_master_id) + _abort_if_configured
    media_player.py            # platform setup (dynamic add/remove) + OndaireMediaPlayer entity
    browse_media.py            # flat /api/media → path trie BrowseMedia; presets; resolve_play_uri
    strings.json               # config-flow strings (cannot_connect, already_configured, ...)
    translations/en.json       # mirror of strings.json
```

## Key implementation details

**Coordinator (push).** `DataUpdateCoordinator[Snapshot]` with `update_interval=None`. On
setup: `GET /api/status` (learn `self_id`), `GET /api/cluster` (seed), start a long-lived WS
task on the shared `aiohttp` session. Each `{type:"cluster"}` frame → parse `Snapshot` →
`async_set_updated_data`. Rebuild a **roster** `{node_id: [origins from addrs[]+httpPort]}`
from every snapshot; on WS close/timeout, backoff then **rotate to another master's origin**
(playback-only nodes have no HTTP) and re-resolve `self_id`. Store position + a monotonic
timestamp at receipt so the entity can extrapolate `media_position`.

**Entity — `OndaireMediaPlayer(CoordinatorEntity, MediaPlayerEntity)`**, one per node,
`_attr_unique_id = node_id`, one HA device per node (`sw_version=appVersion`, model = master
vs player). All properties read live from `coordinator.data` (the node + `group_of(node_id)`).

| Feature flag | Endpoint (via `base(t)` on the connected origin) |
|---|---|
| PLAY / PAUSE / STOP / NEXT_TRACK | `/resume` · `/pause` · `/stop` · `/next` on group master |
| SEEK | `/seek {positionSec}`; gate on `playback.seekable`, catch 409 `not_seekable` |
| VOLUME_SET | normal: `PATCH base(node)/node {volume}` · playback: `POST /api/playback/patch {node,volume}` |
| VOLUME_MUTE | emulated (no mute field): store pre-mute volume, set 0 / restore |
| GROUPING | join: follow(node,leader.node_id) / playback-patch {following}; unjoin: /unfollow or {following:""} |
| BROWSE_MEDIA / PLAY_MEDIA / MEDIA_ENQUEUE | browse `/api/media`+presets; `/play {uri}` or `/queue {uris}` per enqueue |

- **Target resolution `t`:** master-capable entity → `t = node.id`; **playback-only entity →
  transport/media `t = group.master`** (the room's source); volume/name/join use the
  master-side `playback/patch` path with `node = node.id`. Mirror `web/src/lib/api.js`.
- **State:** `playback.state` playing/paused/idle → PLAYING/PAUSED/IDLE; unavailable when node
  absent or `stale`/`!alive` or WS down. No OFF state.
- **Now playing:** title/artist/album/duration from `metadata`; `media_position` +
  `media_position_updated_at` from the captured timestamp. Cover: use `metadata.artUrl`
  (`media_image_remotely_accessible=True`) if set, else implement `async_get_media_image()` to
  proxy `GET base(master)/cover?uri=<playback.uri>` bytes through HA.
- **`group_members`:** entity_ids of `group.members[]`, **master first** (HA leader convention).
  Maintain a node_id→entity_id map in the platform.

**Config flow.** Zeroconf: accept only adverts whose `role` contains `master` (playback-only
nodes have no HTTP/WS to seed from). Fetch `/api/cluster`, set flow unique_id to
`smallest_master_id` (deterministic per cluster regardless of which master answered),
`_abort_if_unique_id_configured` → a second master of the same cluster de-dups. Manual step:
host:port, probe `/api/status`. Entities are keyed by `node_id` so even a stray duplicate entry
can't double-create speakers.

**Browse.** Root children: **Library** (build a trie from the flat `/api/media` list, split on
`/`; dirs `can_expand`, files `can_play` with `media_content_id="file:<rel>"`) and **Stream
presets** (`snapshot.streamPresets[]` → `media_content_id="stream:<id>"`). `async_play_media`
normalizes the id (`file:`/`stream:`/`http(s)://`/`spotify`/`input:` pass through; bare rel →
`file:`), resolves the target master, and honors `enqueue`: PLAY/REPLACE → `/play`, ADD/NEXT →
`/queue`. Cache the flat media list per master with a short TTL (no media revision in snapshot).

**Distribution.** `hacs.json` with `content_in_root:false`; add a `.gitlab-ci.yml` job to zip
`custom_components/ondaire` on tag for HACS/manual install. README documents HACS-custom-repo
(via a GitHub mirror) and copy-in manual install.

## Deferred (later, not v1)
- `outputDelayMs`/`channel` as number/select; group codec/transport/bufferMs settings.
- Calibration start/stop + test tone as buttons; by-ear alignment flow.
- Per-player sync telemetry (`/api/playback/statuses`, sink/clock stats) as diagnostic sensors.
- `SELECT_SOURCE` (line-in / presets / Spotify); stream-preset create/delete services; rename.

## Verification (end-to-end, once implemented)

1. **Run ondaire locally:** `./ondaire` with `MEDIA_DIR=./testdata` (or `data/media`) on
   :8080; optionally a second master on another port + `./player` for a playback node. Confirm
   `curl localhost:8080/api/cluster` and `websocat ws://localhost:8080/api/ws` show frames.
2. **Run HA against it:** official `ghcr.io/home-assistant/home-assistant` (or a venv `hass`)
   with a config dir whose `custom_components/ondaire` bind-mounts this repo's component. Add
   via UI (zeroconf auto-discovers on host-network docker, else manual host). Enable
   `logger: logs: custom_components.ondaire: debug`.
3. **Functional checks:** one entity per node; states track `/api/cluster`; play a file from
   the browser → snapshot flips to `playing`, metadata+cover render; volume slider ↔ `PATCH
   /node` (and ESP32 volume via `playback/patch`); HA join/unjoin ↔ `follow`/`unfollow` +
   `members[]`; seek on a file works, radio surfaces `not_seekable` gracefully; kill the
   connected master → coordinator fails over to another master, dead node goes unavailable.
4. **Unit tests** (`pytest-homeassistant-custom-component` + `aioresponses` + fake WS): feed
   real `/api/cluster` JSON captured as fixtures; assert entity state/attrs; test the browse
   trie + `resolve_play_uri`; test config-flow dedup with two masters; test the
   playback-node-vs-normal mutation branching.
