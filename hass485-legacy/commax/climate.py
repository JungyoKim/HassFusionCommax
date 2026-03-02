"""Platform for Commax climate integration."""
from __future__ import annotations

import logging
from typing import Any
import asyncio

from homeassistant.components.climate import (
    ClimateEntity,
    ClimateEntityFeature,
    HVACMode,
    HVACAction,
)
from homeassistant.config_entries import ConfigEntry
from homeassistant.const import (
    ATTR_TEMPERATURE,
    UnitOfTemperature,
)
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.typing import ConfigType, DiscoveryInfoType

from .const import DOMAIN, BOILER_COUNT
from .hass485_client import HASS485Client

_LOGGER = logging.getLogger(__name__)

# 보일러 이름 정의
BOILER_NAMES = [
    "거실 보일러",
    "안방 보일러",
    "공부방 보일러", 
    "침대방 보일러"
]

async def async_setup_platform(
    hass: HomeAssistant,
    config: ConfigType,
    async_add_entities: AddEntitiesCallback,
    discovery_info: DiscoveryInfoType | None = None,
) -> None:
    """Set up the Commax Climate platform."""
    # This will be called when the integration is set up via configuration.yaml
    # For now, we'll only support config entries
    pass


async def async_setup_entry(
    hass: HomeAssistant,
    config_entry: ConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up Commax Climate based on a config entry."""
    config = hass.data[DOMAIN][config_entry.entry_id]
    
    # HASS485Client 생성 및 연결
    hass485_client = HASS485Client()
    connected = await hass485_client.connect()
    if not connected:
        _LOGGER.warning("HASS485Client 연결 실패 - 엔티티는 생성되지만 기능이 제한됩니다")
    
    # Create climate entities for each boiler
    climates = []
    for i in range(BOILER_COUNT):
        climate = CommaxClimate(
            f"commax_boiler_{i+1}",
            BOILER_NAMES[i],
            config.get("boiler_device", "/dev/ttyUSB1"),
            i + 1,
            hass485_client,
        )
        climates.append(climate)
    
    async_add_entities(climates)


class CommaxClimate(ClimateEntity):
    """Representation of a Commax Boiler."""

    def __init__(self, unique_id: str, name: str, device_path: str, boiler_number: int, hass485_client: HASS485Client) -> None:
        """Initialize the climate entity."""
        self._unique_id = unique_id
        self._name = name
        self._device_path = device_path
        self._boiler_number = boiler_number
        self._hass485_client = hass485_client
        self._hvac_mode = HVACMode.OFF
        self._hvac_action = HVACAction.OFF
        self._current_temperature = 20.0
        self._target_temperature = 20.0
        self._available = False
        
        # 상태 구독 설정
        self._setup_subscriptions()

    def _setup_subscriptions(self) -> None:
        """Setup state subscriptions."""
        # 모드 상태 구독
        mode_path = f"/boilers/{self._boiler_number}/mode"
        asyncio.create_task(
            self._hass485_client.subscribe(mode_path, self._handle_mode_update)
        )
        
        # 현재 온도 구독
        current_temp_path = f"/boilers/{self._boiler_number}/current_temp"
        asyncio.create_task(
            self._hass485_client.subscribe(current_temp_path, self._handle_current_temp_update)
        )
        
        # 설정 온도 구독
        set_temp_path = f"/boilers/{self._boiler_number}/set_temp"
        asyncio.create_task(
            self._hass485_client.subscribe(set_temp_path, self._handle_set_temp_update)
        )
        
        _LOGGER.info("보일러 %d 상태 구독 설정 완료", self._boiler_number)

    async def _handle_mode_update(self, path: str, value: str) -> None:
        """Handle mode update from Go server."""
        _LOGGER.debug("보일러 %d 모드 업데이트: %s -> %s", self._boiler_number, path, value)
        
        if value == "heat":
            self._hvac_mode = HVACMode.HEAT
            self._hvac_action = HVACAction.HEATING
        else:
            self._hvac_mode = HVACMode.OFF
            self._hvac_action = HVACAction.OFF
        
        self._available = True
        self.async_write_ha_state()

    async def _handle_current_temp_update(self, path: str, value: str) -> None:
        """Handle current temperature update from Go server."""
        _LOGGER.debug("보일러 %d 현재 온도 업데이트: %s -> %s", self._boiler_number, path, value)
        
        try:
            # 16진수 문자열을 10진수로 변환
            temp_hex = int(value, 16)
            self._current_temperature = float(temp_hex)
            self._available = True
            self.async_write_ha_state()
        except ValueError as e:
            _LOGGER.error("보일러 %d 온도 파싱 에러: %s", self._boiler_number, e)

    async def _handle_set_temp_update(self, path: str, value: str) -> None:
        """Handle set temperature update from Go server."""
        _LOGGER.debug("보일러 %d 설정 온도 업데이트: %s -> %s", self._boiler_number, path, value)
        
        try:
            # 16진수 문자열을 10진수로 변환
            temp_hex = int(value, 16)
            self._target_temperature = float(temp_hex)
            self._available = True
            self.async_write_ha_state()
        except ValueError as e:
            _LOGGER.error("보일러 %d 설정 온도 파싱 에러: %s", self._boiler_number, e)

    @property
    def unique_id(self) -> str:
        """Return the unique ID of the climate entity."""
        return self._unique_id

    @property
    def name(self) -> str:
        """Return the name of the climate entity."""
        return self._name

    @property
    def icon(self) -> str:
        """Return the icon of the climate entity."""
        return "mdi:water-boiler"

    @property
    def temperature_unit(self) -> str:
        """Return the unit of measurement."""
        return UnitOfTemperature.CELSIUS

    @property
    def current_temperature(self) -> float | None:
        """Return the current temperature."""
        return self._current_temperature

    @property
    def target_temperature(self) -> float | None:
        """Return the temperature we try to reach."""
        return self._target_temperature

    @property
    def target_temperature_step(self) -> float | None:
        """Return the supported step of target temperature."""
        return 0.5

    @property
    def hvac_mode(self) -> HVACMode:
        """Return hvac operation mode."""
        return self._hvac_mode

    @property
    def hvac_action(self) -> HVACAction:
        """Return hvac action."""
        return self._hvac_action

    @property
    def hvac_modes(self) -> list[HVACMode]:
        """Return the list of available hvac operation modes."""
        return [HVACMode.OFF, HVACMode.HEAT]

    @property
    def supported_features(self) -> ClimateEntityFeature:
        """Return the list of supported features."""
        return ClimateEntityFeature.TARGET_TEMPERATURE

    @property
    def available(self) -> bool:
        """Return True if entity is available."""
        return self._available and self._hass485_client.is_connected

    async def async_set_temperature(self, **kwargs: Any) -> None:
        """Set new target temperature."""
        temperature = kwargs.get(ATTR_TEMPERATURE)
        if temperature is not None:
            _LOGGER.info("보일러 %d 온도 설정: %s", self._boiler_number, temperature)
            
            # 1. 먼저 제어 명령 전송
            path = f"/boilers/{self._boiler_number}/temperature/set"
            # 온도를 16진수로 변환
            temp_hex = hex(int(temperature))[2:].upper()
            success = await self._hass485_client.set_state(path, temp_hex)
            
            if success:
                _LOGGER.info("보일러 %d 온도 설정 명령 전송 성공", self._boiler_number)
                # 2. 제어 성공 시 즉시 상태 반영 (UI 반응성 향상)
                self._target_temperature = temperature
                self.async_write_ha_state()
            else:
                _LOGGER.error("보일러 %d 온도 설정 명령 전송 실패", self._boiler_number)

    async def async_set_hvac_mode(self, hvac_mode: HVACMode) -> None:
        """Set new target hvac mode."""
        _LOGGER.info("보일러 %d HVAC 모드 설정: %s", self._boiler_number, hvac_mode)
        
        # 1. 먼저 제어 명령 전송
        path = f"/boilers/{self._boiler_number}/mode/set"
        mode_value = "heat" if hvac_mode == HVACMode.HEAT else "off"
        success = await self._hass485_client.set_state(path, mode_value)
        
        if success:
            _LOGGER.info("보일러 %d HVAC 모드 설정 명령 전송 성공", self._boiler_number)
            # 2. 제어 성공 시 즉시 상태 반영 (UI 반응성 향상)
            self._hvac_mode = hvac_mode
            if hvac_mode == HVACMode.HEAT:
                self._hvac_action = HVACAction.HEATING
            else:
                self._hvac_action = HVACAction.OFF
            self.async_write_ha_state()
        else:
            _LOGGER.error("보일러 %d HVAC 모드 설정 명령 전송 실패", self._boiler_number)

    async def async_update(self) -> None:
        """Fetch new state data for this climate entity."""
        # 초기 상태 로드 (연결 시 한 번만)
        if not self._available:
            # 모드 상태 로드
            mode_path = f"/boilers/{self._boiler_number}/mode"
            mode = await self._hass485_client.get_state(mode_path)
            if mode == "heat":
                self._hvac_mode = HVACMode.HEAT
                self._hvac_action = HVACAction.HEATING
            else:
                self._hvac_mode = HVACMode.OFF
                self._hvac_action = HVACAction.OFF
            
            # 온도 상태 로드
            current_temp_path = f"/boilers/{self._boiler_number}/current_temp"
            current_temp = await self._hass485_client.get_state(current_temp_path)
            if current_temp:
                try:
                    temp_hex = int(current_temp, 16)
                    self._current_temperature = float(temp_hex)
                except ValueError:
                    pass
            
            set_temp_path = f"/boilers/{self._boiler_number}/set_temp"
            set_temp = await self._hass485_client.get_state(set_temp_path)
            if set_temp:
                try:
                    temp_hex = int(set_temp, 16)
                    self._target_temperature = float(temp_hex)
                except ValueError:
                    pass
            
            self._available = True
            _LOGGER.info("보일러 %d 초기 상태 로드 완료", self._boiler_number) 