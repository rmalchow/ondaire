"""Constants for the ondaire integration."""

from __future__ import annotations

from homeassistant.const import CONF_HOST, CONF_PORT  # re-exported for the package
from homeassistant.const import Platform

DOMAIN = "ondaire"
PLATFORMS = [Platform.MEDIA_PLAYER]

# Bump on any ondaire-card.js change so the companion app's WebView cache busts.
CARD_VERSION = "6"

DEFAULT_PORT = 8080

API_PATH = "/api"
WS_PATH = "/api/ws"

# mDNS service (matches internal/discovery: ServiceName "_ondaire._tcp").
ZEROCONF_TYPE = "_ondaire._tcp.local."

# All-zero id sentinel (internal/id.Zero) — "no group" / self marker.
ZERO_ID = "0" * 32

# WS reconnect backoff, mirrored from web/src/lib/ws.svelte.js.
BACKOFF_START_S = 0.5
BACKOFF_MAX_S = 5.0
CONNECT_TIMEOUT_S = 4.0

# Per-master media-list cache TTL (no media revision in the snapshot).
MEDIA_TTL_S = 30.0

# Extra config-entry keys (CONF_HOST / CONF_PORT come from homeassistant.const).
CONF_SELF_ID = "self_id"
CONF_ROSTER = "roster"

# Dispatcher signal fired by the coordinator so the platform can add speakers for
# nodes that join after setup. Formatted per config entry.
SIGNAL_ADD_ENTITIES = DOMAIN + "_add_entities_{}"

__all__ = [
    "CONF_HOST",
    "CONF_PORT",
    "CONF_ROSTER",
    "CONF_SELF_ID",
    "DEFAULT_PORT",
    "DOMAIN",
    "PLATFORMS",
]
