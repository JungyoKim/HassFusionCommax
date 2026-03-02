"""Platform for Commax switch integration."""
from __future__ import annotations

import logging
from typing import Any
import asyncio

from homeassistant.components.switch import SwitchEntity
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.typing import ConfigType, DiscoveryInfoType

from .const import DOMAIN, MASTER_SWITCH_COUNT
from .hass485_client import HASS485Client

_LOGGER = logging.getLogger(__name__)

# 스위치 이름 정의
MASTER_SWITCH_NAMES = ["일괄소등"]

async def async_setup_platform(
    hass: HomeAssistant,
    config: ConfigType,
    async_add_entities: AddEntitiesCallback,
    discovery_info: DiscoveryInfoType | None = None,
) -> None:
    """Set up the Commax Switch platform."""
    # This will be called when the integration is set up via configuration.yaml
    # For now, we'll only support config entries
    pass


async def async_setup_entry(
    hass: HomeAssistant,
    config_entry: ConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up Commax Switch based on a config entry."""
    config = hass.data[DOMAIN][config_entry.entry_id]
    
    # HASS485Client 생성 및 연결
    hass485_client = HASS485Client()
    connected = await hass485_client.connect()
    if not connected:
        _LOGGER.warning("HASS485Client 연결 실패 - 엔티티는 생성되지만 기능이 제한됩니다")
    
    # Create switch entities
    switches = []
    
    # Master switch (일괄소등)
    for i in range(MASTER_SWITCH_COUNT):
        master_switch = CommaxMasterSwitch(
            f"commax_master_switch_{i+1}",
            MASTER_SWITCH_NAMES[i],
            config.get("elevator_device", "/dev/ttyUSB3"),
            i + 1,
            hass485_client,
        )
        switches.append(master_switch)
    
    async_add_entities(switches)


class CommaxMasterSwitch(SwitchEntity):
    """Representation of a Commax Master Switch (일괄소등)."""

    def __init__(self, unique_id: str, name: str, device_path: str, switch_number: int, hass485_client: HASS485Client) -> None:
        """Initialize the master switch."""
        self._unique_id = unique_id
        self._name = name
        self._device_path = device_path
        self._switch_number = switch_number
        self._hass485_client = hass485_client
        self._is_on = False
        self._available = False
        
        # 상태 구독 설정
        self._setup_subscription()

    def _setup_subscription(self) -> None:
        """Setup state subscription."""
        path = "/alloff/state"
        asyncio.create_task(
            self._hass485_client.subscribe(path, self._handle_state_update)
        )
        _LOGGER.info("일괄소등 상태 구독 설정: %s", path)

    async def _handle_state_update(self, path: str, value: str) -> None:
        """Handle state update from Go server."""
        _LOGGER.debug("일괄소등 상태 업데이트: %s -> %s", path, value)
        
        # 상태 반영
        self._is_on = (value == "ON")
        self._available = True
        
        # Home Assistant UI 업데이트
        self.async_write_ha_state()

    @property
    def unique_id(self) -> str:
        """Return the unique ID of the switch."""
        return self._unique_id

    @property
    def name(self) -> str:
        """Return the name of the switch."""
        return self._name

    @property
    def is_on(self) -> bool:
        """Return true if the switch is on."""
        return self._is_on

    @property
    def available(self) -> bool:
        """Return True if entity is available."""
        return self._available and self._hass485_client.is_connected

    async def async_turn_on(self, **kwargs: Any) -> None:
        """Turn the switch on (일괄소등)."""
        _LOGGER.info("일괄소등 ON 명령 실행")
        
        # 1. 먼저 제어 명령 전송
        path = "/alloff/set"
        success = await self._hass485_client.set_state(path, "ON")
        
        if success:
            _LOGGER.info("일괄소등 ON 명령 전송 성공")
            # 2. 제어 성공 시 즉시 상태 반영 (UI 반응성 향상)
            self._is_on = True
            self.async_write_ha_state()
        else:
            _LOGGER.error("일괄소등 ON 명령 전송 실패")

    async def async_turn_off(self, **kwargs: Any) -> None:
        """Turn the switch off (일괄소등 해제)."""
        _LOGGER.info("일괄소등 OFF 명령 실행")
        
        # 1. 먼저 제어 명령 전송
        path = "/alloff/set"
        success = await self._hass485_client.set_state(path, "OFF")
        
        if success:
            _LOGGER.info("일괄소등 OFF 명령 전송 성공")
            # 2. 제어 성공 시 즉시 상태 반영 (UI 반응성 향상)
            self._is_on = False
            self.async_write_ha_state()
        else:
            _LOGGER.error("일괄소등 OFF 명령 전송 실패")

    async def async_update(self) -> None:
        """Fetch new state data for this switch."""
        # 초기 상태 로드 (연결 시 한 번만)
        if not self._available:
            path = "/alloff/state"
            state = await self._hass485_client.get_state(path)
            if state is not None:
                self._is_on = (state == "ON")
                self._available = True
                _LOGGER.info("일괄소등 초기 상태 로드: %s", state) 