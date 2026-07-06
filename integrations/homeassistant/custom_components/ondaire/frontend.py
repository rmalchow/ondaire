"""Auto-registers the ondaire Lovelace card so no manual resource step is needed."""

from __future__ import annotations

import logging
from pathlib import Path

from homeassistant.components.frontend import add_extra_js_url
from homeassistant.components.http import StaticPathConfig
from homeassistant.core import HomeAssistant

from .const import CARD_VERSION, DOMAIN

_LOGGER = logging.getLogger(__name__)

URL_BASE = f"/{DOMAIN}_static"
CARD_FILENAME = "ondaire-card.js"
# Version query busts the companion app's WebView cache on upgrades.
CARD_URL = f"{URL_BASE}/{CARD_FILENAME}?v={CARD_VERSION}"
_REGISTERED = f"{DOMAIN}_frontend_registered"


async def async_register_frontend(hass: HomeAssistant) -> None:
    """Serve www/ and make the card load in every frontend context.

    Idempotent: safe to call once per config entry. Belt-and-suspenders load:
    `add_extra_js_url` covers most desktop setups, and a Lovelace *resource*
    covers the rest — notably the companion app, whose WebView does not reliably
    pick up extra-module URLs but does load dashboard resources.
    """
    if hass.data.get(_REGISTERED):
        return
    hass.data[_REGISTERED] = True

    www_dir = Path(__file__).parent / "www"
    await hass.http.async_register_static_paths(
        [StaticPathConfig(URL_BASE, str(www_dir), cache_headers=False)]
    )
    add_extra_js_url(hass, CARD_URL)
    await _async_register_resource(hass, CARD_URL)


async def _async_register_resource(hass: HomeAssistant, url: str) -> None:
    """Register the card as a Lovelace module resource (storage mode only).

    No-op in YAML dashboard mode (the user adds the resource in YAML themselves)
    or if the lovelace data isn't ready. Defensive across HA versions."""
    lovelace = hass.data.get("lovelace")
    resources = getattr(lovelace, "resources", None)
    if resources is None or not hasattr(resources, "async_create_item"):
        _LOGGER.debug("ondaire: Lovelace resources unavailable (YAML mode?); "
                      "add the card resource manually if needed")
        return
    try:
        if not resources.loaded:
            await resources.async_load()
            resources.loaded = True
        bare = url.split("?", 1)[0]
        for item in resources.async_items():
            if item.get("url", "").split("?", 1)[0] == bare:
                # Keep the version query current on upgrades.
                if item.get("url") != url:
                    await resources.async_update_item(item["id"], {"url": url})
                return
        await resources.async_create_item({"res_type": "module", "url": url})
        _LOGGER.debug("ondaire: registered Lovelace resource %s", url)
    except Exception as err:  # noqa: BLE001 - never block setup on this
        _LOGGER.warning("ondaire: could not auto-register Lovelace resource: %s", err)
