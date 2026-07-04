# ondaire — Home Assistant integration

A Home Assistant custom integration that controls [ondaire](../../), the
multi-room synchronized-audio daemon. It talks to a **room (master) node's**
HTTP API and mirrors live cluster state from that node's WebSocket feed, so
adding one node reaches the whole cluster.

Every ondaire node — a room (`master`) or a player (`playback`, e.g. an ESP32
speaker) — becomes **one `media_player` entity**. Grouping uses HA's
join/unjoin, mapped to ondaire follow/unfollow (the Sonos/HEOS convention).

## What it does

- **Playback**: play / pause / resume / stop / next / seek (seek is gated to
  seekable sources; live streams surface *not seekable* gracefully).
- **Volume**: per-node volume slider; mute is emulated (ondaire has no mute
  field — the pre-mute level is restored on unmute). ESP32 player volume is set
  master-side via `playback/patch`.
- **Grouping**: join a room into another room's group (follow) / unjoin
  (unfollow). `group_members` lists the group master first.
- **Browse & play**: browse the media library (a path tree built from the flat
  `/api/media` list) and saved stream presets, then play or enqueue.
- **Now playing**: title / artist / album / duration / position, plus cover art
  (remote `artUrl` when present, else the master's `/cover` endpoint).
- **Live push**: state tracks the cluster over the WebSocket; if the connected
  master goes away the integration fails over to another master automatically.

## Installation

### Manual (recommended for a self-hosted daemon)

Copy the component into your Home Assistant config directory:

```
<config>/custom_components/ondaire/
```

i.e. copy `custom_components/ondaire` from this folder to
`<config>/custom_components/ondaire`, then restart Home Assistant. On a tagged
release the CI publishes a ready-made `ondaire-hacs.zip` artifact whose paths
start with `custom_components/ondaire/` — unzip it into your `<config>`
directory and restart.

### HACS (optional — for automatic updates)

HACS pulls from a GitHub repository, so use the GitHub mirror
(`github.com/rmalchow/ondaire`) as a **custom repository**:

1. HACS → ⋮ → *Custom repositories* → add the repo URL, category *Integration*.
2. Install **ondaire**, then restart Home Assistant.

`hacs.json` declares `content_in_root: false`, so HACS looks under
`custom_components/`.

> Future: this may be submitted to Home Assistant core, at which point it would
> ship built in and this custom install would no longer be needed.

## Configuration

The integration is set up from the UI (`config_flow`), no YAML.

- **Auto-discovery**: master nodes advertise over mDNS (`_ondaire._tcp`), so HA
  discovers them and offers a one-click *Add*. (Player-only nodes have no API
  and are intentionally ignored as entry points; they still appear as entities
  once a master is added.)
- **Manual**: *Settings → Devices & services → Add integration → ondaire*, then
  enter the host and port of any room node (default port **8080**).

A cluster is de-duplicated by its lowest master id, so discovering a second
master of the same cluster won't create a duplicate entry.

## Requirements

- Ondaire ≥ the release matching this component's `version`, reachable on the
  local network (no auth — trusted LAN).
- Nothing to install: the component uses only Home Assistant-provided libraries.

## Debugging

```yaml
logger:
  logs:
    custom_components.ondaire: debug
```
