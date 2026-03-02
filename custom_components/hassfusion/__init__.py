"""The HassFusion integration."""
import logging

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant

from .const import DOMAIN
from .hub import HassFusionHub

_LOGGER = logging.getLogger(__name__)

PLATFORMS: list[str] = ["light", "climate", "binary_sensor", "sensor", "button", "switch"]

async def async_setup_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Set up HassFusion from a config entry."""
    hub = HassFusionHub(hass, entry.data["host"], entry.data["port"])
    
    # Initialize the Websocket connection
    if not await hub.connect():
        _LOGGER.error("Failed to connect to HassFusion WebSocket Server")
        return False

    hass.data.setdefault(DOMAIN, {})[entry.entry_id] = hub

    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)

    return True

async def async_unload_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Unload a config entry."""
    unload_ok = await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
    
    if unload_ok:
        hub = hass.data[DOMAIN].pop(entry.entry_id)
        await hub.disconnect()

    return unload_ok
