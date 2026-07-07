"""One media_player per ondaire node (room or player)."""

from __future__ import annotations

from typing import Any

from homeassistant.components.media_player import (
    BrowseMedia,
    MediaPlayerEnqueue,
    MediaPlayerEntity,
    MediaPlayerEntityFeature,
    MediaPlayerState,
    MediaType,
)
from homeassistant.core import HomeAssistant, callback
from homeassistant.exceptions import HomeAssistantError
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.dispatcher import async_dispatcher_connect
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from . import OndaireConfigEntry
from .api import OndaireApiError
from .browse_media import async_browse_media, resolve_play_uri, search_results
from .const import DOMAIN, SIGNAL_ADD_ENTITIES
from .coordinator import OndaireCoordinator
from .images import scale_cover
from .models import GroupView, NodeView

# SEARCH_MEDIA + SearchMedia/SearchMediaQuery landed in recent HA cores. Import
# defensively so the integration still loads on older versions; when absent the
# feature bit is 0 (a no-op in _SUPPORT) and async_search_media is never called.
try:
    from homeassistant.components.media_player import SearchMedia, SearchMediaQuery

    _SEARCH_FEATURE = MediaPlayerEntityFeature.SEARCH_MEDIA
except (ImportError, AttributeError):  # pragma: no cover - version shim
    SearchMedia = None  # type: ignore[assignment,misc]
    SearchMediaQuery = None  # type: ignore[assignment,misc]
    _SEARCH_FEATURE = MediaPlayerEntityFeature(0)

_SUPPORT = (
    MediaPlayerEntityFeature.PLAY
    | MediaPlayerEntityFeature.PAUSE
    | MediaPlayerEntityFeature.STOP
    | MediaPlayerEntityFeature.NEXT_TRACK
    | MediaPlayerEntityFeature.SEEK
    | MediaPlayerEntityFeature.VOLUME_SET
    | MediaPlayerEntityFeature.VOLUME_MUTE
    | MediaPlayerEntityFeature.GROUPING
    | MediaPlayerEntityFeature.BROWSE_MEDIA
    | MediaPlayerEntityFeature.PLAY_MEDIA
    | MediaPlayerEntityFeature.MEDIA_ENQUEUE
    | _SEARCH_FEATURE
)

_STATE_MAP = {
    "playing": MediaPlayerState.PLAYING,
    "paused": MediaPlayerState.PAUSED,
    "idle": MediaPlayerState.IDLE,
}


async def async_setup_entry(
    hass: HomeAssistant,
    entry: OndaireConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up media players and add speakers for nodes as they appear."""
    coordinator = entry.runtime_data
    known: set[str] = set()

    @callback
    def _discover() -> None:
        snap = coordinator.data
        if not snap:
            return
        new = [n.id for n in snap.nodes if n.id and n.id not in known]
        known.update(new)
        if new:
            async_add_entities(OndaireMediaPlayer(coordinator, nid) for nid in new)

    entry.async_on_unload(
        async_dispatcher_connect(
            hass, SIGNAL_ADD_ENTITIES.format(entry.entry_id), _discover
        )
    )
    _discover()


class OndaireMediaPlayer(CoordinatorEntity[OndaireCoordinator], MediaPlayerEntity):
    """A single ondaire node as an HA media_player."""

    _attr_has_entity_name = False
    _attr_media_content_type = MediaType.MUSIC
    _attr_supported_features = _SUPPORT

    def __init__(self, coordinator: OndaireCoordinator, node_id: str) -> None:
        super().__init__(coordinator)
        self._node_id = node_id
        self._attr_unique_id = node_id
        self._premute_volume: float | None = None

    async def async_added_to_hass(self) -> None:
        await super().async_added_to_hass()
        self.coordinator.entities[self._node_id] = self

    async def async_will_remove_from_hass(self) -> None:
        self.coordinator.entities.pop(self._node_id, None)
        await super().async_will_remove_from_hass()

    # --- live views ----------------------------------------------------------
    @property
    def _node(self) -> NodeView | None:
        snap = self.coordinator.data
        return snap.node(self._node_id) if snap else None

    @property
    def _group(self) -> GroupView | None:
        snap = self.coordinator.data
        return snap.group_of(self._node_id) if snap else None

    @property
    def _target(self) -> str:
        """The group master that owns transport/media for this node.

        A room masters its own group (master == node.id); a follower or a
        playback-only node routes to its group's master (the source). Unifying
        on group.master covers both — a following room still controls its group.
        """
        grp = self._group
        return grp.master if grp and grp.master else self._node_id

    # --- identity ------------------------------------------------------------
    @property
    def name(self) -> str | None:
        node = self._node
        return node.name if node and node.name else self._node_id[:8]

    @property
    def device_info(self) -> DeviceInfo:
        node = self._node
        return DeviceInfo(
            identifiers={(DOMAIN, self._node_id)},
            name=(node.name if node and node.name else self._node_id[:8]),
            manufacturer="ondaire",
            model="Player" if node and node.playback_node else "Room",
            sw_version=node.app_version if node else None,
        )

    @property
    def available(self) -> bool:
        # `stale` is deliberately NOT part of this: it's a "not updated
        # recently" UI hint (contracts.go), and in a multi-master cluster peer
        # masters are routinely stale on whichever master we're connected to.
        # The web UI treats alive nodes as live regardless; mirror that.
        node = self._node
        return bool(self.coordinator.ws_connected and node and node.alive)

    # --- state ---------------------------------------------------------------
    @property
    def state(self) -> MediaPlayerState | None:
        grp = self._group
        if not grp:
            return MediaPlayerState.IDLE
        return _STATE_MAP.get(grp.playback.state, MediaPlayerState.IDLE)

    @property
    def volume_level(self) -> float | None:
        node = self._node
        if not node:
            return None
        return max(0.0, min(1.0, node.volume))

    @property
    def is_volume_muted(self) -> bool:
        return self._premute_volume is not None

    # --- now playing ---------------------------------------------------------
    def _metadata(self):
        grp = self._group
        return grp.playback.metadata if grp else None

    @property
    def media_title(self) -> str | None:
        md = self._metadata()
        return md.title if md else None

    @property
    def media_artist(self) -> str | None:
        md = self._metadata()
        return md.artist or None if md else None

    @property
    def media_album_name(self) -> str | None:
        md = self._metadata()
        return md.album or None if md else None

    @property
    def media_duration(self) -> int | None:
        md = self._metadata()
        return md.duration_sec or None if md else None

    @property
    def media_position(self) -> int | None:
        grp = self._group
        if not grp or grp.playback.state != "playing":
            return None
        return int(grp.playback.position_sec)

    @property
    def media_position_updated_at(self):
        return self.coordinator.position_ts

    @property
    def media_content_id(self) -> str | None:
        grp = self._group
        return grp.playback.uri or None if grp else None

    def _image_url(self) -> str | None:
        """Source URL for the current cover, or None. Not handed to clients —
        used for HA's cache hash and fetched server-side in async_get_media_image."""
        grp = self._group
        if not grp:
            return None
        md = grp.playback.metadata
        if md and md.art_url:
            return md.art_url  # remote (e.g. Spotify)
        if md and md.has_art and grp.playback.uri:
            return self.coordinator.client.cover_url(self._target, grp.playback.uri)
        return None

    @property
    def media_image_url(self) -> str | None:
        # HA hashes this for the entity_picture cache-buster and to know an image
        # exists; the bytes themselves are fetched by async_get_media_image below.
        return self._image_url()

    @property
    def media_image_remotely_accessible(self) -> bool:
        # Always proxy through HA — never hand the ondaire /cover URL or a remote
        # artUrl to the frontend, which may not be able to reach either.
        return False

    async def async_get_media_image(self) -> tuple[bytes | None, str | None]:
        url = self._image_url()
        if not url:
            return None, None
        try:
            raw, ctype = await self.coordinator.client.fetch_image(url)
        except OndaireApiError:
            return None, None
        # Downscale to a small thumbnail before handing it on. Blocking (PIL
        # decode) → run in the executor.
        scaled = await self.hass.async_add_executor_job(scale_cover, raw)
        if scaled is not None:
            return scaled, "image/jpeg"
        return raw, ctype  # unscalable (no Pillow / bad bytes): pass through

    @property
    def extra_state_attributes(self) -> dict[str, Any]:
        # `ondaire_playback` lets the Lovelace card list the nodes that are
        # speakers (its roster). A node qualifies if it is a wire-driven
        # playback node (D50 satellite — the master holds no probed caps for it,
        # so capabilities.playback reads false) OR it has playback capability
        # (a normal/dual-role node that can output audio, e.g. "study"). The
        # union restores joined satellites while still excluding room-only
        # masters (both false). Mirrors the web UI's roster, which lists group
        # members unconditionally and uses capabilities only for addable nodes.
        node = self._node
        is_player = bool(node) and (node.playback_node or node.playback_capable)
        return {"ondaire_playback": is_player}

    @property
    def group_members(self) -> list[str] | None:
        grp = self._group
        if not grp:
            return None
        # HA leader convention: master first, then the rest.
        ordered = [grp.master] + [m for m in grp.members if m != grp.master]
        out = []
        for node_id in ordered:
            ent = self.coordinator.entities.get(node_id)
            if ent is not None and ent.entity_id:
                out.append(ent.entity_id)
        return out

    # --- transport -----------------------------------------------------------
    async def async_media_play(self) -> None:
        await self._call(self.coordinator.client.resume(self._target))

    async def async_media_pause(self) -> None:
        await self._call(self.coordinator.client.pause(self._target))

    async def async_media_stop(self) -> None:
        await self._call(self.coordinator.client.stop(self._target))

    async def async_media_next_track(self) -> None:
        await self._call(self.coordinator.client.next(self._target))

    async def async_media_seek(self, position: float) -> None:
        grp = self._group
        if not grp or not grp.playback.seekable:
            raise HomeAssistantError("This source can't be seeked")
        await self._call(self.coordinator.client.seek(self._target, position))

    # --- volume --------------------------------------------------------------
    async def async_set_volume_level(self, volume: float) -> None:
        node = self._node
        if node:
            await self._call(self.coordinator.client.set_volume(node, volume))

    async def async_mute_volume(self, mute: bool) -> None:
        node = self._node
        if not node:
            return
        if mute:
            # ondaire has no mute field — emulate by stashing + zeroing volume.
            self._premute_volume = node.volume
            await self._call(self.coordinator.client.set_volume(node, 0.0))
        else:
            restore = self._premute_volume if self._premute_volume is not None else 0.0
            self._premute_volume = None
            await self._call(self.coordinator.client.set_volume(node, restore))
        self.async_write_ha_state()

    # --- grouping ------------------------------------------------------------
    async def async_join_players(self, group_members: list[str]) -> None:
        leader = self._target
        for entity_id in group_members:
            node = self._node_for_entity(entity_id)
            if node:
                await self._call(self.coordinator.client.set_following(node, leader))

    async def async_unjoin_player(self) -> None:
        node = self._node
        if node:
            await self._call(self.coordinator.client.set_following(node, ""))

    # --- media ---------------------------------------------------------------
    async def async_browse_media(
        self,
        media_content_type: str | None = None,
        media_content_id: str | None = None,
    ) -> BrowseMedia:
        return await async_browse_media(
            self.coordinator, self._target, media_content_id
        )

    async def async_search_media(self, query: SearchMediaQuery) -> SearchMedia:
        """Search this room's library (§6), returning playable BrowseMedia hits.

        Only invoked by cores that advertise SEARCH_MEDIA (the feature bit is 0
        otherwise), so SearchMedia/SearchMediaQuery are guaranteed importable here.
        """
        try:
            files = await self.coordinator.client.search_media(
                self._target, query.search_query, limit=100
            )
        except OndaireApiError as err:
            raise HomeAssistantError(str(err)) from err
        return SearchMedia(result=search_results(files))

    async def async_play_media(
        self,
        media_type: str,
        media_id: str,
        enqueue: MediaPlayerEnqueue | None = None,
        announce: bool | None = None,
        **kwargs: Any,
    ) -> None:
        uri = resolve_play_uri(media_id)
        target = self._target
        if enqueue in (MediaPlayerEnqueue.ADD, MediaPlayerEnqueue.NEXT):
            # ponytail: ondaire's queue only appends — NEXT is treated as ADD
            # (no insert-next endpoint). Upgrade path: /queue with an index.
            await self._call(self.coordinator.client.enqueue(target, [uri]))
        else:
            await self._call(self.coordinator.client.play(target, uri))

    # --- queue (driven by the card's Queue tab via websocket) ----------------
    async def async_queue_list(self) -> list[dict]:
        """Upcoming tracks for this room's group, flattened for the frontend."""
        items = await self.coordinator.client.get_queue(self._target)
        out: list[dict] = []
        for it in items:
            md = it.get("metadata") if isinstance(it, dict) else None
            md = md if isinstance(md, dict) else {}
            out.append(
                {
                    "uri": it.get("uri", "") if isinstance(it, dict) else "",
                    "title": md.get("title", ""),
                    "artist": md.get("artist", ""),
                    "album": md.get("album", ""),
                }
            )
        return out

    async def async_queue_remove(self, index: int, uri: str = "") -> None:
        await self._call(self.coordinator.client.queue_remove(self._target, index, uri))

    async def async_queue_play(self, index: int, uri: str = "") -> None:
        await self._call(self.coordinator.client.queue_play(self._target, index, uri))

    async def async_enqueue_dir(self, content_id: str) -> int:
        """Append every file under a library directory to the queue (recursive).

        content_id is the card's browse id: "library" (whole library) or
        "dir:<rel>/". The library is a flat path list, so a prefix match gives
        the full recursive set. Returns the count enqueued."""
        if content_id.startswith("dir:"):
            prefix = content_id[len("dir:") :]
            if prefix and not prefix.endswith("/"):
                prefix += "/"
        else:  # "library" / "root" / anything → whole library
            prefix = ""
        files = await self.coordinator.async_get_media(self._target)
        uris = [f"file:{f.path}" for f in files if f.path.startswith(prefix)]
        if uris:
            await self._call(self.coordinator.client.enqueue(self._target, uris))
        return len(uris)

    async def async_search_list(self, query: str) -> list[dict]:
        """Search this room's library for the card: matching folders first
        (derived from the flat path list — the server search is file-only), then
        tag-aware file hits. Folders are expandable and enqueue-able."""
        q = query.strip().lower()
        out: list[dict] = []

        # Folders: any directory whose leaf name contains the query. Built from
        # the (cached) full library, so it's independent of the tag index.
        all_files = await self.coordinator.async_get_media(self._target)
        dir_leaf: dict[str, str] = {}
        for f in all_files:
            parts = f.path.split("/")
            for i in range(1, len(parts)):  # every ancestor directory
                dir_leaf["/".join(parts[:i])] = parts[i - 1]
        matched_dirs = sorted(
            (p for p, leaf in dir_leaf.items() if q in leaf.lower()),
            key=str.lower,
        )
        for prefix in matched_dirs[:50]:
            out.append(
                {
                    "media_content_id": f"dir:{prefix}/",
                    "media_content_type": "directory",
                    "title": dir_leaf[prefix],
                    "can_expand": True,
                    "can_play": False,
                }
            )

        # Files: the server's tag-aware search (name/path + metadata).
        files = await self.coordinator.client.search_media(
            self._target, query, limit=100
        )
        for f in files:
            path = f.get("path")
            if not path:
                continue
            title = f.get("title") or f.get("name") or path
            artist = f.get("artist")
            out.append(
                {
                    "media_content_id": f"file:{path}",
                    "media_content_type": "music",
                    "title": f"{artist} — {title}" if artist else title,
                    "can_expand": False,
                    "can_play": True,
                }
            )
        return out

    # --- helpers -------------------------------------------------------------
    def _node_for_entity(self, entity_id: str) -> NodeView | None:
        for node_id, ent in self.coordinator.entities.items():
            if ent.entity_id == entity_id:
                return self.coordinator.data.node(node_id) if self.coordinator.data else None
        return None

    async def _call(self, coro) -> None:
        """Await an API coroutine, surfacing failures as HA errors."""
        try:
            await coro
        except OndaireApiError as err:
            raise HomeAssistantError(str(err)) from err
