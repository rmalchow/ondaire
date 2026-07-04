"""The ondaire multi-room audio integration."""

from __future__ import annotations

from homeassistant.config_entries import ConfigEntry
from homeassistant.const import CONF_HOST, CONF_PORT
from homeassistant.core import HomeAssistant
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from .api import OndaireClient
from .const import PLATFORMS
from .coordinator import OndaireCoordinator

type OndaireConfigEntry = ConfigEntry[OndaireCoordinator]


async def async_setup_entry(hass: HomeAssistant, entry: OndaireConfigEntry) -> bool:
    """Set up ondaire from a config entry."""
    session = async_get_clientsession(hass)
    origin = f"http://{entry.data[CONF_HOST]}:{entry.data[CONF_PORT]}"
    client = OndaireClient(session, origin)
    coordinator = OndaireCoordinator(hass, entry, client)
    await coordinator.async_setup()  # raises ConfigEntryNotReady if unreachable

    entry.runtime_data = coordinator
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: OndaireConfigEntry) -> bool:
    """Unload a config entry."""
    unloaded = await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
    if unloaded:
        await entry.runtime_data.async_shutdown()
    return unloaded
