"""Platform for button integration."""
import logging
from typing import Any

from homeassistant.components.button import ButtonEntity
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .const import DOMAIN
from .hub import HassFusionHub

_LOGGER = logging.getLogger(__name__)

async def async_setup_entry(
    hass: HomeAssistant,
    config_entry: ConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up the HassFusion Button platform."""
    hub: HassFusionHub = hass.data[DOMAIN][config_entry.entry_id]

    buttons = [
        # Lobby Door Buttons
        HassFusionButton(hub, "door_B4", "button", "지하 4층 공동현관 열기", "press", "mdi:door-open"),
        HassFusionButton(hub, "door_B3", "button", "지하 3층 공동현관 열기", "press", "mdi:door-open"),
        HassFusionButton(hub, "door_1F", "button", "1층 공동현관 열기", "press", "mdi:door-open"),

        # RS485 Front Door and Elevator Buttons
        HassFusionButton(hub, "doorbell", "doorbell_button", "현관문 열기", "press", "mdi:door-open"),
        HassFusionButton(hub, "elevator_call", "elevator_button", "엘리베이터 호출", "press", "mdi:elevator"),
    ]

    async_add_entities(buttons)

class HassFusionButton(ButtonEntity):
    """Representation of a HassFusion Button."""

    def __init__(self, hub: HassFusionHub, device_id: str, domain: str, name: str, action: str, icon: str) -> None:
        """Initialize."""
        self._hub = hub
        self._device_id = device_id
        self._domain = domain
        self._action = action

        self._attr_name = name
        self._attr_unique_id = f"hassfusion_{device_id}"
        self._attr_icon = icon
        self._unsub_avail: Any = None

    @property
    def available(self) -> bool:
        """Return True if the hub is connected."""
        return self._hub.connected

    async def async_added_to_hass(self) -> None:
        """Register availability callback."""
        self._unsub_avail = self._hub.register_availability_callback(self._handle_availability)

    async def async_will_remove_from_hass(self) -> None:
        """Unregister callbacks."""
        if self._unsub_avail:
            self._unsub_avail()

    def _handle_availability(self, available: bool) -> None:
        """Handle connection state changes."""
        self.schedule_update_ha_state()

    async def async_press(self) -> None:
        """Handle the button press."""
        await self._hub.send_command(self._domain, self._device_id, self._action)
