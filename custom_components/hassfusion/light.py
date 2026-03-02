"""Platform for light integration."""
import logging
from typing import Any

from homeassistant.components.light import LightEntity
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .const import DOMAIN
from .hub import HassFusionHub

_LOGGER = logging.getLogger(__name__)

async def async_setup_entry(
    hass: HomeAssistant,
    config_entry: ConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up the HassFusion Light platform."""
    hub: HassFusionHub = hass.data[DOMAIN][config_entry.entry_id]

    # Typically, we'd query the Hub for existing devices.
    # For now, we statically create the 5 defined lights.
    lights = []
    for i in range(1, 6):
        lights.append(HassFusionLight(hub, f"light_{i}", f"거실 조명 {i}", "mdi:ceiling-light"))

    async_add_entities(lights)

class HassFusionLight(LightEntity):
    """Representation of a HassFusion Light."""

    def __init__(self, hub: HassFusionHub, device_id: str, name: str, icon: str = "mdi:lightbulb") -> None:
        """Initialize."""
        self._hub = hub
        self._device_id = device_id
        self._attr_name = name
        self._attr_unique_id = f"hassfusion_{device_id}"
        self._attr_icon = icon
        self._attr_is_on = False

    async def async_added_to_hass(self) -> None:
        """Register callbacks."""
        self._hub.subscribe(self._device_id, self._handle_event)

    @callback
    def _handle_event(self, event: dict) -> None:
        """Handle state updates from Go Daemon."""
        state = event.get("state")
        self._attr_is_on = (state == "on")
        self.async_write_ha_state()

    async def async_turn_on(self, **kwargs: Any) -> None:
        """Turn the light on."""
        await self._hub.send_command("light", self._device_id, "turn_on")
        self._attr_is_on = True
        self.async_write_ha_state()

    async def async_turn_off(self, **kwargs: Any) -> None:
        """Turn the light off."""
        await self._hub.send_command("light", self._device_id, "turn_off")
        self._attr_is_on = False
        self.async_write_ha_state()
