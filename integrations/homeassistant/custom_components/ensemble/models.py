"""Frozen dataclasses mirroring internal/contracts/contracts.go.

Field names/JSON keys are the single source of truth in the Go contract; these
models track that exactly. `from_json` reads defensively (unknown keys ignored,
missing keys defaulted) so a newer daemon never breaks parsing.
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(frozen=True, slots=True)
class TrackMetadata:
    """contracts.TrackMetadata — optional now-playing info."""

    title: str = ""
    artist: str = ""
    album: str = ""
    art_url: str = ""
    duration_sec: int = 0
    has_art: bool = False

    @classmethod
    def from_json(cls, d: dict) -> "TrackMetadata":
        return cls(
            title=d.get("title", ""),
            artist=d.get("artist", ""),
            album=d.get("album", ""),
            art_url=d.get("artUrl", ""),
            duration_sec=int(d.get("durationSec", 0) or 0),
            has_art=bool(d.get("hasArt", False)),
        )


@dataclass(frozen=True, slots=True)
class Playback:
    """contracts.Playback — the group's replicated playback record."""

    state: str = "idle"
    uri: str = ""
    started_unix: int = 0
    position_sec: float = 0.0
    codec: str = ""
    transport: str = ""
    seekable: bool = False
    queue_len: int = 0
    metadata: TrackMetadata | None = None

    @classmethod
    def from_json(cls, d: dict) -> "Playback":
        md = d.get("metadata")
        return cls(
            state=d.get("state", "idle"),
            uri=d.get("uri", ""),
            started_unix=int(d.get("startedAt", 0) or 0),
            position_sec=float(d.get("positionSec", 0.0) or 0.0),
            codec=d.get("codec", ""),
            transport=d.get("transport", ""),
            seekable=bool(d.get("seekable", False)),
            queue_len=int(d.get("queueLen", 0) or 0),
            metadata=TrackMetadata.from_json(md) if isinstance(md, dict) else None,
        )


@dataclass(frozen=True, slots=True)
class NodeView:
    """contracts.NodeView — one node record (subset used by the integration)."""

    id: str = ""
    name: str = ""
    volume: float = 0.0
    output_delay_ms: int = 0
    channel: str = "stereo"
    addrs: tuple[str, ...] = ()
    http_port: int = 0
    playback_node: bool = False
    following: str = ""
    alive: bool = False
    stale: bool = False
    app_version: str = ""

    @classmethod
    def from_json(cls, d: dict) -> "NodeView":
        return cls(
            id=d.get("id", ""),
            name=d.get("name", ""),
            volume=float(d.get("volume", 0.0) or 0.0),
            output_delay_ms=int(d.get("outputDelayMs", 0) or 0),
            channel=d.get("channel", "stereo"),
            addrs=tuple(d.get("addrs") or ()),
            http_port=int(d.get("httpPort", 0) or 0),
            playback_node=bool(d.get("playbackNode", False)),
            following=d.get("following", ""),
            alive=bool(d.get("alive", False)),
            stale=bool(d.get("stale", False)),
            app_version=d.get("appVersion", ""),
        )


@dataclass(frozen=True, slots=True)
class GroupView:
    """contracts.GroupView — a derived group (id == master node id, D42)."""

    id: str = ""
    name: str = ""
    master: str = ""
    members: tuple[str, ...] = ()
    playback: Playback = field(default_factory=Playback)

    @classmethod
    def from_json(cls, d: dict) -> "GroupView":
        pb = d.get("playback")
        return cls(
            id=d.get("id", ""),
            name=d.get("name", ""),
            master=d.get("master", ""),
            members=tuple(d.get("members") or ()),
            playback=Playback.from_json(pb) if isinstance(pb, dict) else Playback(),
        )


@dataclass(frozen=True, slots=True)
class StreamPreset:
    """contracts.StreamPresetView — a saved HTTP stream preset (secrets omitted)."""

    id: str = ""
    name: str = ""
    url: str = ""
    has_auth: bool = False
    auth_scheme: str = ""

    @classmethod
    def from_json(cls, d: dict) -> "StreamPreset":
        return cls(
            id=d.get("id", ""),
            name=d.get("name", ""),
            url=d.get("url", ""),
            has_auth=bool(d.get("hasAuth", False)),
            auth_scheme=d.get("authScheme", ""),
        )


@dataclass(frozen=True, slots=True)
class MediaFile:
    """api.MediaFile — one playable file, rel path under MEDIA_DIR."""

    path: str = ""
    name: str = ""
    size_bytes: int = 0
    mod_time: int = 0

    @classmethod
    def from_json(cls, d: dict) -> "MediaFile":
        return cls(
            path=d.get("path", ""),
            name=d.get("name", ""),
            size_bytes=int(d.get("sizeBytes", 0) or 0),
            mod_time=int(d.get("modTime", 0) or 0),
        )


@dataclass(frozen=True, slots=True)
class Snapshot:
    """contracts.Snapshot — the resolved cluster view behind /api/cluster + WS."""

    nodes: tuple[NodeView, ...] = ()
    groups: tuple[GroupView, ...] = ()
    stream_presets: tuple[StreamPreset, ...] = ()

    @classmethod
    def from_json(cls, d: dict) -> "Snapshot":
        return cls(
            nodes=tuple(NodeView.from_json(n) for n in (d.get("nodes") or [])),
            groups=tuple(GroupView.from_json(g) for g in (d.get("groups") or [])),
            stream_presets=tuple(
                StreamPreset.from_json(s) for s in (d.get("streamPresets") or [])
            ),
        )

    def node(self, node_id: str) -> NodeView | None:
        """Return the node record for node_id, or None."""
        for n in self.nodes:
            if n.id == node_id:
                return n
        return None

    def group_of(self, node_id: str) -> GroupView | None:
        """Return the group whose members include node_id (crosswise model)."""
        for g in self.groups:
            if node_id in g.members:
                return g
        return None

    def masters(self) -> list[NodeView]:
        """Master-capable nodes: everything that isn't a wire-driven player.

        Only masters expose an HTTP/WS API, so these are the reachable origins
        and the candidates for the cluster's dedup key.
        """
        return [n for n in self.nodes if not n.playback_node]

    def smallest_master_id(self) -> str | None:
        """Lowest master node id — a per-cluster dedup key that is stable
        regardless of which master answered the discovery/probe."""
        ids = sorted(n.id for n in self.masters() if n.id)
        return ids[0] if ids else None
