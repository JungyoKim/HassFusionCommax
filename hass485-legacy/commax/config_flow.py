"""Config flow for Commax integration."""
from __future__ import annotations

import logging
from typing import Any

import voluptuous as vol

from homeassistant import config_entries
from homeassistant.core import HomeAssistant
from homeassistant.data_entry_flow import FlowResult
from homeassistant.exceptions import HomeAssistantError

from .const import DOMAIN

_LOGGER = logging.getLogger(__name__)


class CommaxConfigFlow(config_entries.ConfigFlow, domain=DOMAIN):
    """Handle a config flow for Commax."""

    VERSION = 1

    async def async_step_user(
        self, user_input: dict[str, Any] | None = None
    ) -> FlowResult:
        """Handle the initial step."""
        if user_input is None:
            return self.async_show_form(
                step_id="user",
                data_schema=vol.Schema(
                    {
                        vol.Required("name"): str,
                        vol.Required("light_device"): str,
                        vol.Required("boiler_device"): str,
                        vol.Required("door_device"): str,
                        vol.Required("elevator_device"): str,
                    }
                ),
            )

        return self.async_create_entry(
            title=user_input["name"],
            data=user_input,
        )


class CannotConnect(HomeAssistantError):
    """Error to indicate we cannot connect."""


class InvalidAuth(HomeAssistantError):
    """Error to indicate there is invalid auth.""" 