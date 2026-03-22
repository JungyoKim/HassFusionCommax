"""Platform for climate integration."""
import logging
from typing import Any

from homeassistant.components.climate import (
    ClimateEntity,
    ClimateEntityFeature,
    HVACMode,
)
from homeassistant.config_entries import ConfigEntry
from homeassistant.const import UnitOfTemperature
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
    """Set up the HassFusion Climate platform."""
    hub: HassFusionHub = hass.data[DOMAIN][config_entry.entry_id]

    names = ["거실 보일러", "안방 보일러", "서재 보일러", "침실 보일러"]
    boilers = []

    for i, name in enumerate(names, 1):
        boilers.append(HassFusionBoiler(hub, f"boiler_{i}", name))

    async_add_entities(boilers)

class HassFusionBoiler(ClimateEntity):
    """Representation of a HassFusion Boiler."""

    _attr_has_entity_name = True
    _attr_temperature_unit = UnitOfTemperature.CELSIUS
    _attr_hvac_modes = [HVACMode.OFF, HVACMode.HEAT]
    _attr_supported_features = ClimateEntityFeature.TARGET_TEMPERATURE
    _attr_target_temperature_step = 1
    _attr_icon = "mdi:heating-coil"

    _attr_min_temp = 5
    _attr_max_temp = 35

    def __init__(self, hub: HassFusionHub, device_id: str, name: str) -> None:
        """Initialize."""
        self._hub = hub
        self._device_id = device_id
        self._attr_name = name
        self._attr_unique_id = f"hassfusion_{device_id}"

        self._attr_hvac_mode = HVACMode.OFF
        self._attr_current_temperature = 20.0
        self._attr_target_temperature = 20.0
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
        attrs = event.get("attributes", {})

        mode_str = attrs.get("mode", "off")
        if mode_str == "heat":
            self._attr_hvac_mode = HVACMode.HEAT
        else:
            self._attr_hvac_mode = HVACMode.OFF

        self._attr_current_temperature = float(attrs.get("current_temp", self._attr_current_temperature))
        self._attr_target_temperature = float(attrs.get("target_temp", self._attr_target_temperature))

        self.async_write_ha_state()

    async def async_set_hvac_mode(self, hvac_mode: HVACMode) -> None:
        """Set new target hvac mode."""
        mode_str = "off"
        if hvac_mode == HVACMode.HEAT:
            mode_str = "heat"

        await self._hub.send_command("climate", self._device_id, "set_mode", mode_str)

    async def async_set_temperature(self, **kwargs: Any) -> None:
        """Set new target temperature."""
        temperature = kwargs.get("temperature")
        if temperature is not None:
            await self._hub.send_command("climate", self._device_id, "set_temperature", temperature)
