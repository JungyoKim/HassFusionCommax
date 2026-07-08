"""HassFusion WebSocket Hub."""
import asyncio
import json
import logging
from typing import Any, Callable

import aiohttp

from homeassistant.core import HomeAssistant
from homeassistant.exceptions import HomeAssistantError
from homeassistant.helpers.device_registry import DeviceInfo

from .const import DOMAIN

_LOGGER = logging.getLogger(__name__)

RESYNC_INTERVAL = 300  # 5분마다 전체 상태 재동기화


class HassFusionHub:
    """Manages the WebSocket connection to the Go Daemon."""

    def __init__(self, hass: HomeAssistant, host: str, port: int) -> None:
        """Initialize the hub."""
        self._hass = hass
        self._host = host
        self._port = port
        self._url = f"ws://{host}:{port}/ws"

        self._session: aiohttp.ClientSession | None = None
        self._ws: aiohttp.ClientWebSocketResponse | None = None
        self._listeners: dict[str, list[Callable]] = {}
        self._reconnect_task: asyncio.Task | None = None
        self._resync_task: asyncio.Task | None = None
        self._is_closing = False
        self._connected = False
        self._availability_callbacks: list[Callable[[bool], None]] = []

    @property
    def connected(self) -> bool:
        """Return True if currently connected to the Go daemon."""
        return self._connected

    @property
    def device_info(self) -> DeviceInfo:
        """Return device info for the shared HassFusion bridge device."""
        return DeviceInfo(
            identifiers={(DOMAIN, f"{self._host}:{self._port}")},
            name="HassFusion Bridge",
            manufacturer="Commax",
            model="HassFusion",
            configuration_url=None,
        )

    def register_availability_callback(self, callback: Callable[[bool], None]) -> Callable[[], None]:
        """Register a callback for connection state changes. Returns unsubscribe callable."""
        self._availability_callbacks.append(callback)
        def _unsubscribe() -> None:
            self._availability_callbacks.remove(callback)
        return _unsubscribe

    def _notify_availability(self, available: bool) -> None:
        """Notify all entities about connection state change."""
        self._connected = available
        for cb in self._availability_callbacks:
            cb(available)

    async def connect(self) -> bool:
        """Start the background connection loop."""
        self._is_closing = False
        self._session = aiohttp.ClientSession()
        self._reconnect_task = asyncio.create_task(self._connection_loop())
        return True

    async def _connection_loop(self) -> None:
        """Maintain a persistent connection with exponential backoff."""
        retry_delay = 5

        while not self._is_closing:
            try:
                _LOGGER.info("Attempting to connect to HassFusion Go Daemon at %s", self._url)
                self._ws = await self._session.ws_connect(
                    self._url,
                    heartbeat=30,
                )
                _LOGGER.info("Connected to HassFusion Go Daemon at %s", self._url)

                retry_delay = 5
                self._notify_availability(True)

                await self.send_command("system", "hassfusion", "request_sync")

                self._resync_task = asyncio.create_task(self._periodic_resync())

                await self._listen()

            except (aiohttp.ClientError, OSError) as err:
                _LOGGER.warning("Failed to connect to HassFusion: %s. Will retry...", err)
            except asyncio.CancelledError:
                break
            except Exception as err:
                _LOGGER.error("Unexpected error in HassFusion connection loop: %s", err)
            finally:
                self._notify_availability(False)
                if self._resync_task:
                    self._resync_task.cancel()
                    self._resync_task = None

            if not self._is_closing:
                _LOGGER.info("Reconnecting in %d seconds...", retry_delay)
                await asyncio.sleep(retry_delay)
                retry_delay = min(retry_delay * 2, 60)

    async def _periodic_resync(self) -> None:
        """Periodically request full state sync from Go daemon."""
        while True:
            await asyncio.sleep(RESYNC_INTERVAL)
            if self._ws and not self._ws.closed:
                _LOGGER.debug("Periodic resync: requesting full state from Go daemon")
                await self.send_command("system", "hassfusion", "request_sync")

    async def disconnect(self) -> None:
        """Disconnect from the server permanently."""
        self._is_closing = True
        if self._resync_task:
            self._resync_task.cancel()
        if self._reconnect_task:
            self._reconnect_task.cancel()
        if self._ws and not self._ws.closed:
            await self._ws.close()
        if self._session and not self._session.closed:
            await self._session.close()
        self._notify_availability(False)

    def subscribe(self, device_id: str, callback: Callable) -> Callable[[], None]:
        """Subscribe to events for a specific device. Returns unsubscribe callable."""
        if device_id not in self._listeners:
            self._listeners[device_id] = []
        self._listeners[device_id].append(callback)
        def _unsubscribe() -> None:
            self._listeners[device_id].remove(callback)
            if not self._listeners[device_id]:
                del self._listeners[device_id]
        return _unsubscribe

    async def send_command(self, domain: str, device_id: str, action: str, value: Any = None) -> None:
        """Send a command to the Go Daemon."""
        if not self._ws or self._ws.closed:
            raise HomeAssistantError("HassFusion 데몬에 연결되어 있지 않습니다")

        payload = {
            "type": "command",
            "domain": domain,
            "device_id": device_id,
            "action": action,
        }
        if value is not None:
            payload["value"] = value

        await self._ws.send_json(payload)
        _LOGGER.debug("Sent command: %s", payload)

    async def _listen(self) -> None:
        """Listen for incoming WebSocket messages."""
        if not self._ws:
            return

        try:
            async for msg in self._ws:
                if msg.type == aiohttp.WSMsgType.TEXT:
                    try:
                        data = msg.json()
                        _LOGGER.debug("Received event: %s", data)

                        if data.get("type") == "event":
                            device_id = data.get("device_id")
                            if device_id in self._listeners:
                                for callback in list(self._listeners[device_id]):
                                    self._hass.loop.call_soon(callback, data)

                    except json.JSONDecodeError:
                        _LOGGER.warning("Received invalid JSON from HassFusion")

                elif msg.type == aiohttp.WSMsgType.CLOSED:
                    _LOGGER.warning("WebSocket connection closed by server")
                    break
                elif msg.type == aiohttp.WSMsgType.ERROR:
                    _LOGGER.error("WebSocket error")
                    break

        except asyncio.CancelledError:
            pass
        except Exception as err:
            _LOGGER.error("Unexpected error in WebSocket listener: %s", err)
