# HTTP API, WebSocket & UI

The control surface every node serves: a REST API, a WebSocket state feed, a
transparent node proxy, and the embedded Svelte web UI. All under `/api`, JSON, no
auth (trusted LAN, v1). Built on Echo.

For the end-user tour of the UI screens, see [`user/ui-reference.md`](../user/ui-reference.md).

## REST

| Method / path | Action |
|---|---|
| `GET  /api/status` | this node: id, name, ports, role, group, sink/clock stats, and — when this node runs a source — `source: {clients, connects, restarts, primes}` |
| `PATCH /api/node` | `{name?, volume?, outputDelayMs?, outputDevice?, disabled?}` → rename / set live volume / set output-delay calibration (re-anchors playout) / select the ALSA output device (live backend swap) / toggle operator-disabled features |
| `GET  /api/cluster` | replicated state, resolved: nodes (alive/dead, addrs, observed), derived groups (id, name, master, members, playback, source stats) |
| `GET  /api/media` | list this node's local media |
| `POST /api/follow` | `{target}` → this node follows target |
| `POST /api/unfollow` | this node becomes a solo master |
| `POST /api/group/name` | `{group, name}` → set a group's name override (any node may write); stored under the group's current member-set XOR. An empty `name` clears it, reverting to the derived label |
| `POST /api/group/master` | `{node}` → takeover |
| `POST /api/play` | `{uri}` (back-compat: `{file}` ≡ a `file:` URI) → master only: serve that source to the group. Non-masters reject with a hint to use takeover |
| `POST /api/stop` | master only: stop group playback |
| `POST /api/pause` | master only: freeze the running session; 409 when nothing is playing |
| `POST /api/resume` | master only: resume a paused session; 409 when not paused |
| `GET/POST /api/group/settings` | `{codec, transport, bufferMs}` for this node's group (master only for POST; applies live via RECONFIG) |

Sink stats in `/api/status` include `played, silence, lateDrop, staleGen, synced,
ratePPM, buffered` — the servo's current correction and jitter-buffer depth (see
[playout](playout-pipeline.md)).

## WebSocket

`GET /api/ws` pushes `{type:"cluster", data:<same as GET /api/cluster>}` on every state
change (debounced ≥ 250 ms), plus a periodic (5 s) heartbeat with playback position. The
SPA renders exclusively from these events after the initial load.

## Node proxy

`/api/:nodeId/*` — when the first path segment after `/api/` is a 32-hex node ID (or a
unique node *name*), the request is transparently proxied to that node's HTTP port
(address chosen per the [observed-IP rules](discovery-and-cluster.md#addresses-and-observed-ip-reporting)).
E.g. `GET http://node1:8080/api/<aliceId>/media` lists alice's media via node1. **One
hop only**: proxied requests carry `X-Ondaire-Proxied: 1` and a request with that header
is never re-proxied. This is what lets any node's UI control and browse the whole cluster.

## Web UI (Svelte SPA)

Svelte 5 + Vite, plain JavaScript, no component library, minimal hand-written CSS. Built
to static files, embedded in the binary via `go:embed`, served at `/` by every node. It
connects to `/api/ws` and renders from cluster events; actions are REST calls (proxied
when they target another node).

It is deliberately small — a single page with three sections:

- **Groups / Rooms** — a card per derived group: name (click to rename; derived labels
  render muted/italic), members with master badge and a live volume slider each, playback
  status + position, source stats (listeners / reconnects) on the master, stop button,
  per-member "make master", join via a member's *Join group…* dropdown, *Leave*.
- **Nodes** — all known nodes: name (rename works for remote nodes via proxy), id,
  addresses, capabilities, alive/last-seen, stale indicator, volume slider, and an
  **output delay (ms)** calibration field.
- **Media** — node picker (any node, via proxy) → file list → **Play here** (does
  takeover first if the target isn't its group's master). A URL field plays an
  `http(s)://` stream; an *Input* button plays the node's capture input.
