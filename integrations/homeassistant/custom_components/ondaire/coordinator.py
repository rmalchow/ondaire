"""Push coordinator: mirrors a master's cluster state over its WebSocket feed."""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
import time

import aiohttp

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.exceptions import ConfigEntryNotReady
from homeassistant.helpers.dispatcher import async_dispatcher_send
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator
from homeassistant.util import dt as dt_util

from .api import OndaireApiError, OndaireClient
from .const import (
    BACKOFF_MAX_S,
    BACKOFF_START_S,
    CONNECT_TIMEOUT_S,
    DOMAIN,
    MEDIA_TTL_S,
    SIGNAL_ADD_ENTITIES,
)
from .models import MediaFile, Snapshot

_LOGGER = logging.getLogger(__name__)


class OndaireCoordinator(DataUpdateCoordinator[Snapshot]):
    """DataUpdateCoordinator with `update_interval=None` — state arrives by push.

    Owns a long-lived WS task: each {type:"cluster"} frame becomes a Snapshot;
    on close/timeout it backs off and rotates to another master's origin. Its
    5s heartbeat frame refreshes playback.positionSec, timestamped at receipt.
    """

    def __init__(
        self, hass: HomeAssistant, entry: ConfigEntry, client: OndaireClient
    ) -> None:
        super().__init__(
            hass,
            _LOGGER,
            name=DOMAIN,
            config_entry=entry,
            update_interval=None,  # pushed, never polled
        )
        self.client = client
        self.ws_connected = False
        self.position_ts = dt_util.utcnow()
        # node_id -> [http origins], rebuilt from every snapshot for failover.
        self.roster: dict[str, list[str]] = {}
        # node_id -> entity, populated by the media_player platform for grouping.
        self.entities: dict[str, object] = {}
        self._media_cache: dict[str, tuple[float, list[MediaFile]]] = {}
        self._ws_task: asyncio.Task | None = None

    async def async_setup(self) -> None:
        """Learn self_id, seed state, and start the push task. Raises
        ConfigEntryNotReady if the master can't be reached."""
        try:
            status = await self.client.get_status()
            self.client.self_id = status.get("id", "")
            snap = Snapshot.from_json(await self.client.get_cluster())
        except OndaireApiError as err:
            raise ConfigEntryNotReady(str(err)) from err
        self._apply_snapshot(snap)
        self._ws_task = self.config_entry.async_create_background_task(
            self.hass, self._ws_loop(), f"{DOMAIN}_ws"
        )

    async def async_shutdown(self) -> None:
        if self._ws_task:
            self._ws_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await self._ws_task
            self._ws_task = None
        await super().async_shutdown()

    # --- state ---------------------------------------------------------------
    def _apply_snapshot(self, snap: Snapshot) -> None:
        self.position_ts = dt_util.utcnow()
        self.roster = _build_roster(snap)
        self.async_set_updated_data(snap)
        async_dispatcher_send(
            self.hass, SIGNAL_ADD_ENTITIES.format(self.config_entry.entry_id)
        )

    async def async_get_media(self, master: str) -> list[MediaFile]:
        """Flat media list for a master, cached with a short TTL (no revision
        marker rides the snapshot to invalidate on)."""
        now = time.monotonic()
        cached = self._media_cache.get(master)
        if cached and now - cached[0] < MEDIA_TTL_S:
            return cached[1]
        files = [MediaFile.from_json(x) for x in await self.client.get_media(master)]
        self._media_cache[master] = (now, files)
        return files

    # --- push loop -----------------------------------------------------------
    async def _ws_loop(self) -> None:
        backoff = BACKOFF_START_S
        while True:
            ws = None
            try:
                async with asyncio.timeout(CONNECT_TIMEOUT_S):
                    ws = await self.client.ws_connect()
                self.ws_connected = True
                backoff = BACKOFF_START_S  # reset on a clean open
                async for msg in ws:
                    if msg.type == aiohttp.WSMsgType.TEXT:
                        self._handle_frame(msg.data)
                    elif msg.type in (
                        aiohttp.WSMsgType.ERROR,
                        aiohttp.WSMsgType.CLOSE,
                        aiohttp.WSMsgType.CLOSED,
                    ):
                        break
            except asyncio.CancelledError:
                if ws is not None:
                    await ws.close()
                raise
            except Exception as err:  # noqa: BLE001 - reconnect on anything
                _LOGGER.debug("ondaire WS error: %s", err)
            finally:
                if ws is not None and not ws.closed:
                    with contextlib.suppress(Exception):
                        await ws.close()

            if self.ws_connected:
                self.ws_connected = False
                self.async_update_listeners()  # flip entities unavailable

            await self._failover()
            await asyncio.sleep(backoff)
            backoff = min(backoff * 2, BACKOFF_MAX_S)

    def _handle_frame(self, raw: str) -> None:
        try:
            msg = json.loads(raw)
        except ValueError:
            return
        if isinstance(msg, dict) and msg.get("type") == "cluster" and msg.get("data"):
            self._apply_snapshot(Snapshot.from_json(msg["data"]))

    async def _failover(self) -> None:
        """Rotate to another master's origin (playback nodes have no HTTP) and
        re-resolve self_id. Mirrors the JS roster failover."""
        candidates = [
            o for origins in self.roster.values() for o in origins
        ]
        for origin in candidates:
            if origin == self.client.origin:
                continue
            self.client.origin = origin
            try:
                status = await self.client.get_status()
                self.client.self_id = status.get("id", "")
                self._apply_snapshot(Snapshot.from_json(await self.client.get_cluster()))
                _LOGGER.debug("ondaire failed over to %s", origin)
                return
            except OndaireApiError:
                continue  # try the next origin, else retry current on next loop


def _build_roster(snap: Snapshot) -> dict[str, list[str]]:
    """{node_id: [http origins]} from each master's addrs + httpPort.

    Mirrors originsFor() in ws.svelte.js: strip the CIDR mask, bracket IPv6.
    Playback-only nodes have no HTTP and are excluded by masters().
    """
    roster: dict[str, list[str]] = {}
    for node in snap.masters():
        if not node.http_port:
            continue
        origins: list[str] = []
        for cidr in node.addrs:
            ip = str(cidr).split("/", 1)[0].strip()
            if not ip:
                continue
            host = f"[{ip}]" if ":" in ip else ip
            origins.append(f"http://{host}:{node.http_port}")
        if origins:
            roster[node.id] = origins
    return roster
