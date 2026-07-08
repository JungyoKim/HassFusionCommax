"""Config flow for HassFusion integration."""
import asyncio
import logging
from typing import Any, Dict, Optional

import aiohttp
import voluptuous as vol

from homeassistant import config_entries
from homeassistant.const import CONF_HOST, CONF_PORT
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from .const import DOMAIN, DEFAULT_PORT

_LOGGER = logging.getLogger(__name__)


class HassFusionConfigFlow(config_entries.ConfigFlow, domain=DOMAIN):
    """Handle a config flow for HassFusion."""

    VERSION = 1

    async def async_step_user(
        self, user_input: Optional[Dict[str, Any]] = None
    ) -> config_entries.ConfigFlowResult:
        """Handle the initial step."""
        errors: Dict[str, str] = {}

        if user_input is not None:
            host = user_input[CONF_HOST]
            port = user_input[CONF_PORT]

            await self.async_set_unique_id(f"{host}:{port}")
            self._abort_if_unique_id_configured()

            session = async_get_clientsession(self.hass)
            try:
                async with asyncio.timeout(5):
                    ws = await session.ws_connect(f"ws://{host}:{port}/ws")
                    await ws.close()
            except (aiohttp.ClientError, asyncio.TimeoutError, OSError):
                errors["base"] = "cannot_connect"
            else:
                return self.async_create_entry(
                    title=f"HassFusion ({host})", data=user_input
                )

        data_schema = vol.Schema(
            {
                vol.Required(CONF_HOST): str,
                vol.Required(CONF_PORT, default=DEFAULT_PORT): int,
            }
        )

        return self.async_show_form(
            step_id="user", data_schema=data_schema, errors=errors
        )
