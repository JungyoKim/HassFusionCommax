"""Platform for binary sensor integration."""
import logging
from typing import Any

from homeassistant.components.binary_sensor import (
    BinarySensorDeviceClass,
    BinarySensorEntity,
)
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
    """Set up the HassFusion Binary Sensor platform."""
    hub: HassFusionHub = hass.data[DOMAIN][config_entry.entry_id]

    sensors = [
        HassFusionDoorbell(hub, "doorbell", "현관 도어벨"),

        # Elevator 1 Binary Sensors
        HassFusionGenericBinarySensor(hub, "elevator_1_call_up", "엘리베이터 1호기 상행 호출", "mdi:elevator-up", BinarySensorDeviceClass.PRESENCE),
        HassFusionGenericBinarySensor(hub, "elevator_1_call_down", "엘리베이터 1호기 하행 호출", "mdi:elevator-down", BinarySensorDeviceClass.PRESENCE),
        HassFusionGenericBinarySensor(hub, "elevator_1_basement", "엘리베이터 1호기 지하 여부", "mdi:stairs-down", None),

        # Elevator 2 Binary Sensors
        HassFusionGenericBinarySensor(hub, "elevator_2_call_up", "엘리베이터 2호기 상행 호출", "mdi:elevator-up", BinarySensorDeviceClass.PRESENCE),
        HassFusionGenericBinarySensor(hub, "elevator_2_call_down", "엘리베이터 2호기 하행 호출", "mdi:elevator-down", BinarySensorDeviceClass.PRESENCE),
        HassFusionGenericBinarySensor(hub, "elevator_2_basement", "엘리베이터 2호기 지하 여부", "mdi:stairs-down", None),
    ]
    async_add_entities(sensors)

class HassFusionGenericBinarySensor(BinarySensorEntity):
    """Generic Binary Sensor."""

    def __init__(self, hub: HassFusionHub, device_id: str, name: str, icon: str = None, device_class: str = None) -> None:
        self._hub = hub
        self._attr_device_info = hub.device_info
        self._device_id = device_id

        self._attr_name = name
        self._attr_unique_id = f"hassfusion_binary_sensor_{device_id}"
        if icon:
            self._attr_icon = icon
        if device_class:
            self._attr_device_class = device_class
        self._attr_is_on = False
        self._unsub_event: Any = None
        self._unsub_avail: Any = None

    @property
    def available(self) -> bool:
        """Return True if the hub is connected."""
        return self._hub.connected

    async def async_added_to_hass(self) -> None:
        self._unsub_event = self._hub.subscribe(self._device_id, self._handle_event)
        self._unsub_avail = self._hub.register_availability_callback(self._handle_availability)

    async def async_will_remove_from_hass(self) -> None:
        if self._unsub_event:
            self._unsub_event()
        if self._unsub_avail:
            self._unsub_avail()

    @callback
    def _handle_availability(self, available: bool) -> None:
        self.async_write_ha_state()

    @callback
    def _handle_event(self, event: dict) -> None:
        state = event.get("state")
        self._attr_is_on = (state == "on")
        self.async_write_ha_state()

class HassFusionDoorbell(BinarySensorEntity):
    """Representation of a HassFusion Doorbell Sensor."""

    _attr_device_class = BinarySensorDeviceClass.OCCUPANCY
    _attr_icon = "mdi:bell-ring"

    def __init__(self, hub: HassFusionHub, device_id: str, name: str) -> None:
        """Initialize the sensor."""
        self._hub = hub
        self._attr_device_info = hub.device_info
        self._device_id = device_id

        self._attr_name = name
        self._attr_unique_id = f"hassfusion_binary_sensor_{device_id}"
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
        self.async_write_ha_state()

    @callback
    def _handle_event(self, event: dict) -> None:
        """Handle state updates from Go Daemon."""
        state = event.get("state")

        if state == "on":
            self._attr_is_on = True
        else:
            self._attr_is_on = False

        self.async_write_ha_state()
