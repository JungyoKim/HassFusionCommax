"""HassFusion WebSocket Hub."""
import asyncio
import json
import logging
from typing import Callable, Dict, List

import aiohttp

from homeassistant.core import HomeAssistant

_LOGGER = logging.getLogger(__name__)

class HassFusionHub:
    """Manages the WebSocket connection to the Go Daemon."""

    def __init__(self, hass: HomeAssistant, host: str, port: int) -> None:
        """Initialize the hub."""
        self._hass = hass
        self._host = host
        self._port = port
        self._url = f"ws://{host}:{port}/ws"
        
        self._session = aiohttp.ClientSession()
        self._ws: aiohttp.ClientWebSocketResponse | None = None
        self._listeners: Dict[str, List[Callable]] = {}  # device_id -> callbacks
        self._listen_task: asyncio.Task | None = None
        self._reconnect_task: asyncio.Task | None = None
        self._is_closing = False

    async def connect(self) -> bool:
        """Start the background connection loop."""
        self._is_closing = False
        self._reconnect_task = asyncio.create_task(self._connection_loop())
        # We return True immediately to let HA setup succeed. 
        # The sensor entities will handle unavailable states gracefully.
        return True

    async def _connection_loop(self) -> None:
        """Maintain a persistent connection with exponential backoff."""
        retry_delay = 5
        
        while not self._is_closing:
            try:
                _LOGGER.info("Attempting to connect to HassFusion Go Daemon at %s", self._url)
                self._ws = await self._session.ws_connect(self._url)
                _LOGGER.info("Connected to HassFusion Go Daemon at %s", self._url)
                
                # Reset retry delay on successful connection
                retry_delay = 5
                
                # Request all current hardware states from the Go Daemon
                await self.send_command("system", "hassfusion", "request_sync")
                
                # Block on the listen loop until connection is lost
                await self._listen()
                
            except aiohttp.ClientError as err:
                _LOGGER.error("Failed to connect to HassFusion: %s", err)
            except Exception as err:
                _LOGGER.error("Unexpected error in HassFusion connection loop: %s", err)
                
            if not self._is_closing:
                _LOGGER.info("Reconnecting in %d seconds...", retry_delay)
                await asyncio.sleep(retry_delay)
                # Exponential backoff up to 60 seconds
                retry_delay = min(retry_delay * 2, 60)

    async def disconnect(self) -> None:
        """Disconnect from the server permanently."""
        self._is_closing = True
        if self._reconnect_task:
            self._reconnect_task.cancel()
        if self._ws and not self._ws.closed:
            await self._ws.close()
        if self._session and not self._session.closed:
            await self._session.close()

    def subscribe(self, device_id: str, callback: Callable) -> None:
        """Subscribe to events for a specific device."""
        if device_id not in self._listeners:
            self._listeners[device_id] = []
        self._listeners[device_id].append(callback)

    async def send_command(self, domain: str, device_id: str, action: str, value: any = None) -> None:
        """Send a command to the Go Daemon."""
        if not self._ws or self._ws.closed:
            _LOGGER.error("Cannot send command, WebSocket is closed")
            return

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
                                for callback in self._listeners[device_id]:
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
