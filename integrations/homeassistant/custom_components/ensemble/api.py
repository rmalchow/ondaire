"""HTTP + WebSocket client for a master node's API.

Proxy-aware, mirroring web/src/lib/api.js: a call targeting another node is
issued against /api/<nodeId>/<rest> and the connected master proxies it one hop.
Playback-node mutations are the exception — they are always master-local
(/api/playback/patch), never proxied, because a playback node has no HTTP API.
"""

from __future__ import annotations

import json
from urllib.parse import quote

import aiohttp

from .const import API_PATH, WS_PATH, ZERO_ID
from .models import NodeView


class EnsembleApiError(Exception):
    """A non-2xx response from the ensemble API.

    Carries the machine-stable `code` (body {"error"}) and human `hint` so
    callers can branch (e.g. 409 not_seekable) or surface the message.
    """

    def __init__(self, status: int, code: str = "", hint: str = "") -> None:
        super().__init__(hint or code or f"HTTP {status}")
        self.status = status
        self.code = code
        self.hint = hint


class EnsembleClient:
    """Talks to ONE master origin; reaches the cluster via that node's proxy."""

    def __init__(
        self,
        session: aiohttp.ClientSession,
        origin: str,
        self_id: str = "",
    ) -> None:
        self._session = session
        self.origin = origin.rstrip("/")
        self.self_id = self_id

    @property
    def session(self) -> aiohttp.ClientSession:
        return self._session

    def base(self, node: str | None) -> str:
        """"/api" for self/empty, else "/api/<node>" (mirrors api.js base())."""
        if not node or node == ZERO_ID or node == self.self_id:
            return API_PATH
        return f"{API_PATH}/{node}"

    def ws_url(self) -> str:
        """ws(s):// URL for the connected origin's push feed."""
        scheme = "wss" if self.origin.startswith("https") else "ws"
        host = self.origin.split("://", 1)[-1]
        return f"{scheme}://{host}{WS_PATH}"

    def cover_url(self, master: str, uri: str) -> str:
        """Absolute URL for a file's cover art, proxied to the playing master."""
        return f"{self.origin}{self.base(master)}/cover?uri={quote(uri, safe='')}"

    async def fetch_image(self, url: str) -> tuple[bytes, str | None]:
        """GET raw image bytes + content-type over the shared session.

        Used to pull cover art (local /cover or a remote artUrl) through HA so
        the frontend never has to reach the ensemble host or the art CDN itself.
        """
        async with self._session.get(url) as resp:
            if resp.status >= 400:
                raise EnsembleApiError(resp.status)
            return await resp.read(), resp.content_type

    async def _request(self, method: str, path: str, body: dict | None = None):
        url = f"{self.origin}{path}"
        try:
            async with self._session.request(method, url, json=body) as resp:
                text = await resp.text()
                data = None
                if text:
                    try:
                        data = json.loads(text)
                    except ValueError:
                        data = None
                if resp.status >= 400:
                    code = hint = ""
                    if isinstance(data, dict):
                        code = data.get("error", "")
                        hint = data.get("hint", "")
                    raise EnsembleApiError(resp.status, code, hint)
                return data
        except aiohttp.ClientError as err:
            raise EnsembleApiError(0, "network_error", str(err)) from err

    # --- reads ---------------------------------------------------------------
    async def get_status(self) -> dict:
        return await self._request("GET", f"{API_PATH}/status")

    async def get_cluster(self) -> dict:
        return await self._request("GET", f"{API_PATH}/cluster")

    async def get_media(self, node: str | None = None) -> list[dict]:
        data = await self._request("GET", f"{self.base(node)}/media")
        return data or []

    # --- transport (group master only) --------------------------------------
    async def play(self, node: str, uri: str) -> None:
        await self._request("POST", f"{self.base(node)}/play", {"uri": uri})

    async def enqueue(self, node: str, uris: list[str]) -> None:
        await self._request("POST", f"{self.base(node)}/queue", {"uris": uris})

    async def pause(self, node: str) -> None:
        await self._request("POST", f"{self.base(node)}/pause")

    async def resume(self, node: str) -> None:
        await self._request("POST", f"{self.base(node)}/resume")

    async def stop(self, node: str) -> None:
        await self._request("POST", f"{self.base(node)}/stop")

    async def next(self, node: str) -> None:
        await self._request("POST", f"{self.base(node)}/next")

    async def seek(self, node: str, position_sec: float) -> None:
        await self._request(
            "POST", f"{self.base(node)}/seek", {"positionSec": position_sec}
        )

    # --- per-node config -----------------------------------------------------
    async def patch_node(self, node: str, fields: dict) -> None:
        await self._request("PATCH", f"{self.base(node)}/node", fields)

    async def follow(self, node: str, target: str) -> None:
        await self._request("POST", f"{self.base(node)}/follow", {"target": target})

    async def unfollow(self, node: str) -> None:
        await self._request("POST", f"{self.base(node)}/unfollow")

    async def patch_playback(self, fields: dict) -> None:
        # NEVER proxied: a playback node has no HTTP API, so this is master-local.
        await self._request("POST", f"{API_PATH}/playback/patch", fields)

    # --- node-aware routers (branch on playbackNode, mirrors api.js) ---------
    async def set_volume(self, node: NodeView, volume: float) -> None:
        if node.playback_node:
            await self.patch_playback({"node": node.id, "volume": volume})
        else:
            await self.patch_node(node.id, {"volume": volume})

    async def set_following(self, node: NodeView, target: str) -> None:
        """target == "" leaves the group; else follow that master."""
        if node.playback_node:
            await self.patch_playback({"node": node.id, "following": target})
        elif target:
            await self.follow(node.id, target)
        else:
            await self.unfollow(node.id)

    def ws_connect(self):
        """Return the aiohttp ws_connect context manager for the push feed.

        heartbeat pings detect a wedged TCP connection (no FIN) so the
        coordinator can fail over rather than hang.
        """
        return self._session.ws_connect(self.ws_url(), heartbeat=15)
