"""Platform for sensor integration."""
import logging
from typing import Any

from homeassistant.components.sensor import SensorEntity
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
    """Set up the HassFusion Sensor platform."""
    hub: HassFusionHub = hass.data[DOMAIN][config_entry.entry_id]

    sensors = [
        HassFusionParkingSensor(hub, "parking_listener", "최근 주차 알림"),

        # Elevator 1 Sensors
        HassFusionSensor(hub, "elevator_1_floor", "엘리베이터 1호기 층수", "mdi:elevator"),
        HassFusionSensor(hub, "elevator_1_direction", "엘리베이터 1호기 방향", "mdi:arrow-up-down"),
        HassFusionSensor(hub, "elevator_1_status", "엘리베이터 1호기 상태", "mdi:check-circle"),

        # Elevator 2 Sensors
        HassFusionSensor(hub, "elevator_2_floor", "엘리베이터 2호기 층수", "mdi:elevator"),
        HassFusionSensor(hub, "elevator_2_direction", "엘리베이터 2호기 방향", "mdi:arrow-up-down"),
        HassFusionSensor(hub, "elevator_2_status", "엘리베이터 2호기 상태", "mdi:check-circle"),

        # Energy Monthly Sensors
        HassFusionSensor(hub, "energy_electricity_monthly", "이번 달 전기 사용량", "mdi:flash", "energy", "total_increasing", "kWh"),
        HassFusionSensor(hub, "energy_gas_monthly", "이번 달 가스 사용량", "mdi:fire", "gas", "total_increasing", "m³"),
        HassFusionSensor(hub, "energy_water_monthly", "이번 달 수도 사용량", "mdi:water", "water", "total_increasing", "m³"),

        # Energy Yearly Sensors
        HassFusionSensor(hub, "energy_electricity_yearly", "올해 전기 사용량", "mdi:flash", "energy", "total_increasing", "kWh"),
        HassFusionSensor(hub, "energy_gas_yearly", "올해 가스 사용량", "mdi:fire", "gas", "total_increasing", "m³"),
        HassFusionSensor(hub, "energy_water_yearly", "올해 수도 사용량", "mdi:water", "water", "total_increasing", "m³"),

        # Energy Daily Sensors
        HassFusionSensor(hub, "energy_electricity_daily", "오늘 전기 사용량", "mdi:flash", "energy", "total_increasing", "kWh"),
        HassFusionSensor(hub, "energy_gas_daily", "오늘 가스 사용량", "mdi:fire", "gas", "total_increasing", "m³"),
        HassFusionSensor(hub, "energy_water_daily", "오늘 수도 사용량", "mdi:water", "water", "total_increasing", "m³"),

        # Energy Hourly Sensors
        HassFusionSensor(hub, "energy_electricity_hourly", "현재 시간 전기 사용량", "mdi:flash", "energy", "total_increasing", "kWh"),
        HassFusionSensor(hub, "energy_gas_hourly", "현재 시간 가스 사용량", "mdi:fire", "gas", "total_increasing", "m³"),
        HassFusionSensor(hub, "energy_water_hourly", "현재 시간 수도 사용량", "mdi:water", "water", "total_increasing", "m³"),
    ]
    async_add_entities(sensors)

class HassFusionSensor(SensorEntity):
    """Representation of a generic HassFusion Sensor."""

    def __init__(self, hub: HassFusionHub, device_id: str, name: str, icon: str, device_class: str = None, state_class: str = None, unit: str = None) -> None:
        """Initialize the generic sensor."""
        self._hub = hub
        self._device_id = device_id

        self._attr_name = name
        self._attr_unique_id = f"hassfusion_sensor_{device_id}"
        self._attr_icon = icon
        if device_class:
            self._attr_device_class = device_class
        if state_class:
            self._attr_state_class = state_class
        if unit:
            self._attr_native_unit_of_measurement = unit

        self._attr_native_value = None
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

        if hasattr(self, "_attr_state_class") and self._attr_state_class == "total_increasing":
            try:
                self._attr_native_value = float(state)
            except (ValueError, TypeError):
                pass
        else:
            self._attr_native_value = state

        self.async_write_ha_state()

class HassFusionParkingSensor(SensorEntity):
    """Representation of a HassFusion Parking Sensor."""

    _attr_icon = "mdi:car"

    def __init__(self, hub: HassFusionHub, device_id: str, name: str) -> None:
        """Initialize the sensor."""
        self._hub = hub
        self._device_id = device_id

        self._attr_name = name
        self._attr_unique_id = f"hassfusion_sensor_{device_id}"
        self._attr_native_value = "Waiting..."
        self._attr_extra_state_attributes = {}
        self._unsub_event: Any = None
        self._unsub_avail: Any = None

    @property
    def available(self) -> bool:
        """Return True if the hub is connected."""
        return self._hub.connected

    async def async_added_to_hass(self) -> None:
        """Register callback for parking events."""
        self._unsub_event = self._hub.subscribe("parking_events_all", self._handle_event)
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
        attrs = event.get("attributes") or {}
        state = event.get("state")

        car_no = attrs.get("car_no") or event.get("car_no", "Unknown")
        timestamp = attrs.get("timestamp") or event.get("timestamp", "")

        state_ko = "입차" if state == "parkIn" else "출차" if state == "parkOut" else state

        self._attr_native_value = f"{car_no} ({state_ko})"
        self._attr_extra_state_attributes = {
            "car": car_no,
            "event": state_ko,
            "time": timestamp
        }

        self.async_write_ha_state()
