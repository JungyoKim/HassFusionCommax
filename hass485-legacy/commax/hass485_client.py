"""Unix Socket client for Home Assistant Commax integration."""
from __future__ import annotations

import asyncio
import json
import logging
import socket
import time
from typing import Any, Callable, Dict, Optional

_LOGGER = logging.getLogger(__name__)


class HASS485Client:
    """Home Assistant용 Unix Socket 클라이언트."""

    def __init__(self, socket_path: str = "/config/hass485.sock"):
        """Initialize the HASS485 client."""
        self.socket_path = socket_path
        self.socket: Optional[socket.socket] = None
        self._connected = False
        self._callbacks: Dict[str, Callable] = {}
        self._receive_task: Optional[asyncio.Task] = None
        self._lock = asyncio.Lock()

    async def connect(self) -> bool:
        """Connect to the Unix Socket server."""
        try:
            self.socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            self.socket.connect(self.socket_path)
            self._connected = True
            
            # 수신 루프 시작
            self._receive_task = asyncio.create_task(self._receive_loop())
            _LOGGER.info("Unix Socket 연결 성공: %s", self.socket_path)
            return True
        except Exception as e:
            _LOGGER.error("Unix Socket 연결 실패: %s", e)
            self._connected = False
            return False

    async def disconnect(self) -> None:
        """Disconnect from the Unix Socket server."""
        self._connected = False
        if self._receive_task:
            self._receive_task.cancel()
        if self.socket:
            self.socket.close()
            self.socket = None

    async def _reconnect(self) -> None:
        """Reconnect to the Unix Socket server."""
        while not self._connected:
            _LOGGER.info("Unix Socket 재연결 시도 중...")
            await asyncio.sleep(5)
            await self.connect()

    async def _receive_loop(self) -> None:
        """Receive messages from the Unix Socket server."""
        buffer = ""
        
        while self._connected:
            try:
                if not self.socket:
                    await self._reconnect()
                    continue

                # 비동기 읽기
                data = await asyncio.get_event_loop().run_in_executor(
                    None, self.socket.recv, 4096
                )
                
                if not data:
                    _LOGGER.warning("Unix Socket 연결이 끊어짐")
                    self._connected = False
                    await self._reconnect()
                    continue

                buffer += data.decode('utf-8')
                
                # 완전한 JSON 메시지들을 처리
                messages = self._parse_messages(buffer)
                buffer = ""
                
                for message in messages:
                    await self._handle_message(message)
                    
            except Exception as e:
                _LOGGER.error("Unix Socket 수신 에러: %s", e)
                self._connected = False
                await self._reconnect()

    def _parse_messages(self, buffer: str) -> list[Dict[str, Any]]:
        """Parse multiple JSON messages from buffer."""
        messages = []
        start = 0
        depth = 0
        
        for i, char in enumerate(buffer):
            if char == '{':
                if depth == 0:
                    start = i
                depth += 1
            elif char == '}':
                depth -= 1
                if depth == 0:
                    # 완전한 JSON 객체 추출
                    json_str = buffer[start:i+1]
                    try:
                        message = json.loads(json_str)
                        messages.append(message)
                    except json.JSONDecodeError as e:
                        _LOGGER.error("JSON 파싱 에러: %s", e)
        
        return messages

    async def _handle_message(self, message: Dict[str, Any]) -> None:
        """Handle received message."""
        msg_type = message.get("type")
        path = message.get("path")
        value = message.get("value")
        
        _LOGGER.debug("메시지 수신: type=%s, path=%s, value=%s", msg_type, path, value)
        
        if msg_type == "PUBLISH" and path:
            # 상태 발행 메시지 처리
            if path in self._callbacks:
                callback = self._callbacks[path]
                try:
                    await callback(path, value)
                except Exception as e:
                    _LOGGER.error("콜백 실행 에러: %s", e)

    async def send_message(self, message: Dict[str, Any], timeout: float = 5.0) -> bool:
        """Send message to the Unix Socket server."""
        async with self._lock:
            if not self._connected:
                _LOGGER.error("Unix Socket이 연결되지 않음")
                return False

            try:
                message_str = json.dumps(message)
                await asyncio.get_event_loop().run_in_executor(
                    None, self.socket.send, message_str.encode('utf-8')
                )
                _LOGGER.debug("메시지 전송: %s", message_str)
                return True
            except Exception as e:
                _LOGGER.error("메시지 전송 실패: %s", e)
                self._connected = False
                return False

    async def subscribe(self, path: str, callback: Callable) -> None:
        """Subscribe to state changes."""
        self._callbacks[path] = callback
        _LOGGER.info("상태 구독 등록: %s", path)

    async def unsubscribe(self, path: str) -> None:
        """Unsubscribe from state changes."""
        if path in self._callbacks:
            del self._callbacks[path]
            _LOGGER.info("상태 구독 해제: %s", path)

    async def get_state(self, path: str) -> Optional[Any]:
        """Get current state."""
        message = {
            "type": "GET",
            "path": path,
            "id": f"get_{int(time.time() * 1000)}"
        }
        
        # 응답을 받기 위한 임시 콜백
        response_received = asyncio.Event()
        response_data = [None]
        
        async def temp_callback(response_path: str, value: Any):
            if response_path == path:
                response_data[0] = value
                response_received.set()
        
        # 임시 구독
        await self.subscribe(path, temp_callback)
        
        # 메시지 전송
        if await self.send_message(message):
            try:
                await asyncio.wait_for(response_received.wait(), timeout=5.0)
                return response_data[0]
            except asyncio.TimeoutError:
                _LOGGER.error("상태 조회 타임아웃: %s", path)
                return None
        else:
            return None

    async def set_state(self, path: str, value: Any) -> bool:
        """Set state (send command)."""
        message = {
            "type": "SET",
            "path": path,
            "value": value,
            "id": f"set_{int(time.time() * 1000)}"
        }
        
        return await self.send_message(message)

    @property
    def is_connected(self) -> bool:
        """Return connection status."""
        return self._connected 