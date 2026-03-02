"""Platform for Commax binary sensor integration."""
from __future__ import annotations

import logging
from typing import Any
import asyncio

from homeassistant.components.binary_sensor import (
    BinarySensorDeviceClass,
    BinarySensorEntity,
)
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.typing import ConfigType, DiscoveryInfoType

from .const import DOMAIN, DOORBELL_COUNT
from .hass485_client import HASS485Client

_LOGGER = logging.getLogger(__name__)

# 센서 이름 정의
DOORBELL_NAMES = ["현관 도어벨"]

async def async_setup_platform(
    hass: HomeAssistant,
    config: ConfigType,
    async_add_entities: AddEntitiesCallback,
    discovery_info: DiscoveryInfoType | None = None,
) -> None:
    """Set up the Commax Binary Sensor platform."""
    # This will be called when the integration is set up via configuration.yaml
    # For now, we'll only support config entries
    pass


async def async_setup_entry(
    hass: HomeAssistant,
    config_entry: ConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up Commax Binary Sensor based on a config entry."""
    config = hass.data[DOMAIN][config_entry.entry_id]
    
    # HASS485Client 생성 및 연결
    hass485_client = HASS485Client()
    connected = await hass485_client.connect()
    if not connected:
        _LOGGER.warning("HASS485Client 연결 실패 - 엔티티는 생성되지만 기능이 제한됩니다")
    
    # Create binary sensor entities
    binary_sensors = []
    
    # Doorbell sensors
    for i in range(DOORBELL_COUNT):
        doorbell_sensor = CommaxDoorbellSensor(
            f"commax_doorbell_{i+1}",
            DOORBELL_NAMES[i],
            config.get("door_device", "/dev/ttyUSB2"),
            i + 1,
            hass485_client,
        )
        binary_sensors.append(doorbell_sensor)
    
    async_add_entities(binary_sensors)


class CommaxDoorbellSensor(BinarySensorEntity):
    """Representation of a Commax Doorbell Sensor."""

    def __init__(self, unique_id: str, name: str, device_path: str, doorbell_number: int, hass485_client: HASS485Client) -> None:
        """Initialize the doorbell sensor."""
        self._unique_id = unique_id
        self._name = name
        self._device_path = device_path
        self._doorbell_number = doorbell_number
        self._hass485_client = hass485_client
        self._is_on = False
        self._available = False
        
        # 상태 구독 설정
        self._setup_subscription()

    def _setup_subscription(self) -> None:
        """Setup state subscription."""
        path = "/doorbell/state"
        asyncio.create_task(
            self._hass485_client.subscribe(path, self._handle_state_update)
        )
        _LOGGER.info("도어벨 상태 구독 설정: %s", path)

    async def _handle_state_update(self, path: str, value: str) -> None:
        """Handle state update from Go server."""
        _LOGGER.debug("도어벨 상태 업데이트: %s -> %s", path, value)
        
        # 상태 반영 (벨 울림 감지)
        self._is_on = (value == "ON")
        self._available = True
        
        # Home Assistant UI 업데이트
        self.async_write_ha_state()

    @property
    def unique_id(self) -> str:
        """Return the unique ID of the sensor."""
        return self._unique_id

    @property
    def name(self) -> str:
        """Return the name of the sensor."""
        return self._name

    @property
    def icon(self) -> str:
        """Return the icon of the sensor."""
        return "mdi:bell-ring"

    @property
    def device_class(self) -> BinarySensorDeviceClass:
        """Return the device class of the sensor."""
        return BinarySensorDeviceClass.OCCUPANCY

    @property
    def is_on(self) -> bool:
        """Return true if the binary sensor is on."""
        return self._is_on

    @property
    def available(self) -> bool:
        """Return True if entity is available."""
        return self._available and self._hass485_client.is_connected

    async def async_update(self) -> None:
        """Fetch new state data for this sensor."""
        # 초기 상태 로드 (연결 시 한 번만)
        if not self._available:
            path = "/doorbell/state"
            state = await self._hass485_client.get_state(path)
            if state is not None:
                self._is_on = (state == "ON")
                self._available = True
                _LOGGER.info("도어벨 초기 상태 로드: %s", state) 