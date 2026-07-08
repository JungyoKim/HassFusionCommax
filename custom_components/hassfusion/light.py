"""Platform for light integration."""
import logging
from typing import Any

from homeassistant.components.light import ColorMode, LightEntity
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

    lights = []
    for i in range(1, 6):
        lights.append(HassFusionLight(hub, f"light_{i}", f"거실 조명 {i}", "mdi:ceiling-light"))

    async_add_entities(lights)

class HassFusionLight(LightEntity):
    """Representation of a HassFusion Light."""

    def __init__(self, hub: HassFusionHub, device_id: str, name: str, icon: str = "mdi:lightbulb") -> None:
        """Initialize."""
        self._hub = hub
        self._attr_device_info = hub.device_info
        self._device_id = device_id
        self._attr_name = name
        self._attr_unique_id = f"hassfusion_{device_id}"
        self._attr_icon = icon
        self._attr_is_on = False
        self._attr_color_mode = ColorMode.ONOFF
        self._attr_supported_color_modes = {ColorMode.ONOFF}
        self._unsub_event: Any = None
        self._unsub_avail: Any = None

    @property
    def available(self) -> bool:
        """Return True if the hub is connected."""
        return self._hub.connected

    async def async_added_to_hass(self) -> None:
        """Register callbacks."""
        self._unsub_event = self._hub.subscribe(self._device_id, self._handle_event)
        self._unsub_avail = self._hub.register_availability_callback(self._handle_availability)

    async def async_will_remove_from_hass(self) -> None:
        """Unregister callbacks."""
        if self._unsub_event:
            self._unsub_event()
        if self._unsub_avail:
            self._unsub_avail()

    @callback
    def _handle_availability(self, available: bool) -> None:
        """Handle connection state changes."""
        self.async_write_ha_state()

    @callback
    def _handle_event(self, event: dict) -> None:
        """Handle state updates from Go Daemon."""
        state = event.get("state")
        self._attr_is_on = (state == "on")
        self.async_write_ha_state()

    async def async_turn_on(self, **kwargs: Any) -> None:
        """Turn the light on."""
        await self._hub.send_command("light", self._device_id, "turn_on")

    async def async_turn_off(self, **kwargs: Any) -> None:
        """Turn the light off."""
        await self._hub.send_command("light", self._device_id, "turn_off")
