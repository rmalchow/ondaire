"""WebSocket commands backing the card's Queue tab.

The queue is not part of the media_player state (it isn't gossiped — see
contracts.go), so the card pulls/mutates it on demand through these commands.
Everything still flows browser → HA → ondaire master, keeping the card's
"HA proxies every request" guarantee.
"""

from __future__ import annotations

from typing import Any

import voluptuous as vol

from homeassistant.components import websocket_api
from homeassistant.core import HomeAssistant, callback

from .api import OndaireApiError
from .const import DOMAIN

_REGISTERED = f"{DOMAIN}_ws_registered"


@callback
def async_register_websocket(hass: HomeAssistant) -> None:
    """Register the ondaire/queue/* commands once per HA instance."""
    if hass.data.get(_REGISTERED):
        return
    hass.data[_REGISTERED] = True
    websocket_api.async_register_command(hass, ws_queue_list)
    websocket_api.async_register_command(hass, ws_queue_remove)
    websocket_api.async_register_command(hass, ws_queue_play)
    websocket_api.async_register_command(hass, ws_search)
    websocket_api.async_register_command(hass, ws_enqueue_dir)


def _find_entity(hass: HomeAssistant, entity_id: str):
    """The OndaireMediaPlayer object for entity_id, or None.

    Walks our config entries' coordinators rather than a private hass.data key,
    so it stays valid across HA refactors."""
    for entry in hass.config_entries.async_entries(DOMAIN):
        coordinator = getattr(entry, "runtime_data", None)
        if not coordinator:
            continue
        for ent in coordinator.entities.values():
            if getattr(ent, "entity_id", None) == entity_id:
                return ent
    return None


async def _resolve(hass, connection, msg):
    """Shared: locate the entity or send a websocket error. Returns entity|None."""
    entity = _find_entity(hass, msg["entity_id"])
    if entity is None:
        connection.send_error(msg["id"], "not_found", "unknown ondaire entity")
        return None
    return entity


@websocket_api.websocket_command(
    {
        vol.Required("type"): "ondaire/queue/list",
        vol.Required("entity_id"): str,
    }
)
@websocket_api.async_response
async def ws_queue_list(hass, connection, msg: dict[str, Any]) -> None:
    entity = await _resolve(hass, connection, msg)
    if entity is None:
        return
    try:
        items = await entity.async_queue_list()
    except OndaireApiError as err:
        connection.send_error(msg["id"], "ondaire_error", str(err))
        return
    connection.send_result(msg["id"], {"items": items})


@websocket_api.websocket_command(
    {
        vol.Required("type"): "ondaire/queue/remove",
        vol.Required("entity_id"): str,
        vol.Required("index"): int,
        vol.Optional("uri", default=""): str,
    }
)
@websocket_api.async_response
async def ws_queue_remove(hass, connection, msg: dict[str, Any]) -> None:
    entity = await _resolve(hass, connection, msg)
    if entity is None:
        return
    try:
        await entity.async_queue_remove(msg["index"], msg["uri"])
    except OndaireApiError as err:
        connection.send_error(msg["id"], "ondaire_error", str(err))
        return
    connection.send_result(msg["id"], {})


@websocket_api.websocket_command(
    {
        vol.Required("type"): "ondaire/queue/play",
        vol.Required("entity_id"): str,
        vol.Required("index"): int,
        vol.Optional("uri", default=""): str,
    }
)
@websocket_api.async_response
async def ws_queue_play(hass, connection, msg: dict[str, Any]) -> None:
    entity = await _resolve(hass, connection, msg)
    if entity is None:
        return
    try:
        await entity.async_queue_play(msg["index"], msg["uri"])
    except OndaireApiError as err:
        connection.send_error(msg["id"], "ondaire_error", str(err))
        return
    connection.send_result(msg["id"], {})


@websocket_api.websocket_command(
    {
        vol.Required("type"): "ondaire/search",
        vol.Required("entity_id"): str,
        vol.Required("query"): str,
    }
)
@websocket_api.async_response
async def ws_search(hass, connection, msg: dict[str, Any]) -> None:
    entity = await _resolve(hass, connection, msg)
    if entity is None:
        return
    try:
        items = await entity.async_search_list(msg["query"])
    except OndaireApiError as err:
        connection.send_error(msg["id"], "ondaire_error", str(err))
        return
    connection.send_result(msg["id"], {"items": items})


@websocket_api.websocket_command(
    {
        vol.Required("type"): "ondaire/enqueue_dir",
        vol.Required("entity_id"): str,
        vol.Required("content_id"): str,
    }
)
@websocket_api.async_response
async def ws_enqueue_dir(hass, connection, msg: dict[str, Any]) -> None:
    entity = await _resolve(hass, connection, msg)
    if entity is None:
        return
    try:
        count = await entity.async_enqueue_dir(msg["content_id"])
    except OndaireApiError as err:
        connection.send_error(msg["id"], "ondaire_error", str(err))
        return
    connection.send_result(msg["id"], {"count": count})
