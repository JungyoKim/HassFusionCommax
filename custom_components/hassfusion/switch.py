"""Platform for switch integration."""
import logging
from typing import Any

from homeassistant.components.switch import SwitchEntity
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
    """Set up the HassFusion Switch platform."""
    hub: HassFusionHub = hass.data[DOMAIN][config_entry.entry_id]

    switches = [
        HassFusionSwitch(hub, "alloff", "일괄 소등", "mdi:powersleep"),
    ]

    async_add_entities(switches)


class HassFusionSwitch(SwitchEntity):
    """Representation of a HassFusion Switch."""

    def __init__(self, hub: HassFusionHub, device_id: str, name: str, icon: str) -> None:
        """Initialize."""
        self._hub = hub
        self._attr_device_info = hub.device_info
        self._device_id = device_id

        self._attr_name = name
        self._attr_unique_id = f"hassfusion_switch_{device_id}"
        self._attr_icon = icon
        self._attr_is_on = False
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
        """Turn the switch on."""
        await self._hub.send_command("switch", self._device_id, "turn_on")

    async def async_turn_off(self, **kwargs: Any) -> None:
        """Turn the switch off."""
        await self._hub.send_command("switch", self._device_id, "turn_off")
