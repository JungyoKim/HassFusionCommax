"""Platform for Commax button integration."""
from __future__ import annotations

import logging
from typing import Any

from homeassistant.components.button import ButtonEntity
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.typing import ConfigType, DiscoveryInfoType

from .const import DOMAIN, DOOR_BUTTON_COUNT, ELEVATOR_BUTTON_COUNT
from .hass485_client import HASS485Client

_LOGGER = logging.getLogger(__name__)

# 버튼 이름 정의
DOOR_BUTTON_NAMES = ["현관문 열기"]
ELEVATOR_BUTTON_NAMES = ["엘리베이터 호출"]

async def async_setup_platform(
    hass: HomeAssistant,
    config: ConfigType,
    async_add_entities: AddEntitiesCallback,
    discovery_info: DiscoveryInfoType | None = None,
) -> None:
    """Set up the Commax Button platform."""
    # This will be called when the integration is set up via configuration.yaml
    # For now, we'll only support config entries
    pass


async def async_setup_entry(
    hass: HomeAssistant,
    config_entry: ConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up Commax Button based on a config entry."""
    config = hass.data[DOMAIN][config_entry.entry_id]
    
    # HASS485Client 생성 및 연결
    hass485_client = HASS485Client()
    connected = await hass485_client.connect()
    if not connected:
        _LOGGER.warning("HASS485Client 연결 실패 - 엔티티는 생성되지만 기능이 제한됩니다")
    
    # Create button entities
    buttons = []
    
    # Door button (현관문 열기)
    for i in range(DOOR_BUTTON_COUNT):
        door_button = CommaxDoorButton(
            f"commax_door_button_{i+1}",
            DOOR_BUTTON_NAMES[i],
            config.get("door_device", "/dev/ttyUSB2"),
            i + 1,
            hass485_client,
        )
        buttons.append(door_button)
    
    # Elevator call buttons
    for i in range(ELEVATOR_BUTTON_COUNT):
        elevator_button = CommaxElevatorButton(
            f"commax_elevator_button_{i+1}",
            ELEVATOR_BUTTON_NAMES[i],
            config.get("elevator_device", "/dev/ttyUSB3"),
            i + 1,
            hass485_client,
        )
        buttons.append(elevator_button)
    
    async_add_entities(buttons)


class CommaxDoorButton(ButtonEntity):
    """Representation of a Commax Door Button (현관문 열기)."""

    def __init__(self, unique_id: str, name: str, device_path: str, door_number: int, hass485_client: HASS485Client) -> None:
        """Initialize the door button."""
        self._unique_id = unique_id
        self._name = name
        self._device_path = device_path
        self._door_number = door_number
        self._hass485_client = hass485_client
        self._available = False

    @property
    def unique_id(self) -> str:
        """Return the unique ID of the button."""
        return self._unique_id

    @property
    def name(self) -> str:
        """Return the name of the button."""
        return self._name

    @property
    def icon(self) -> str:
        """Return the icon of the button."""
        return "mdi:door-open"

    @property
    def available(self) -> bool:
        """Return True if entity is available."""
        return self._hass485_client.is_connected

    async def async_press(self) -> None:
        """Press the button (현관문 열기)."""
        _LOGGER.info("현관문 열기 버튼이 눌렸습니다")
        
        # 제어 명령 전송
        path = "/door/open/set"
        success = await self._hass485_client.set_state(path, "ON")
        
        if success:
            _LOGGER.info("현관문 열기 명령 전송 성공")
        else:
            _LOGGER.error("현관문 열기 명령 전송 실패")


class CommaxElevatorButton(ButtonEntity):
    """Representation of a Commax Elevator Call Button."""

    def __init__(self, unique_id: str, name: str, device_path: str, elevator_number: int, hass485_client: HASS485Client) -> None:
        """Initialize the elevator button."""
        self._unique_id = unique_id
        self._name = name
        self._device_path = device_path
        self._elevator_number = elevator_number
        self._hass485_client = hass485_client
        self._available = False

    @property
    def unique_id(self) -> str:
        """Return the unique ID of the button."""
        return self._unique_id

    @property
    def name(self) -> str:
        """Return the name of the button."""
        return self._name

    @property
    def icon(self) -> str:
        """Return the icon of the button."""
        return "mdi:elevator"

    @property
    def available(self) -> bool:
        """Return True if entity is available."""
        return self._hass485_client.is_connected

    async def async_press(self) -> None:
        """Press the button (엘리베이터 호출)."""
        _LOGGER.info("엘리베이터 호출 버튼이 눌렸습니다")
        
        # 제어 명령 전송
        path = "/elevator/call/set"
        success = await self._hass485_client.set_state(path, "ON")
        
        if success:
            _LOGGER.info("엘리베이터 호출 명령 전송 성공")
        else:
            _LOGGER.error("엘리베이터 호출 명령 전송 실패") 