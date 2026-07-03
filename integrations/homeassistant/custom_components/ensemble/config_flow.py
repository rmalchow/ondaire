"""Config flow for ensemble: zeroconf (masters only) + manual host."""

from __future__ import annotations

from typing import Any

import voluptuous as vol

from homeassistant.config_entries import ConfigFlow, ConfigFlowResult
from homeassistant.const import CONF_HOST, CONF_PORT
from homeassistant.helpers.aiohttp_client import async_get_clientsession
from homeassistant.helpers.service_info.zeroconf import ZeroconfServiceInfo

from .api import EnsembleApiError, EnsembleClient
from .const import DEFAULT_PORT, DOMAIN
from .models import Snapshot


class EnsembleConfigFlow(ConfigFlow, domain=DOMAIN):
    """Handle a config flow for ensemble."""

    def __init__(self) -> None:
        self._host: str = ""
        self._port: int = DEFAULT_PORT

    async def _resolve_cluster_id(self, host: str, port: int) -> str | None:
        """Probe /api/cluster and return the cluster's stable dedup key
        (smallest master id), or None if unreachable."""
        session = async_get_clientsession(self.hass)
        client = EnsembleClient(session, f"http://{host}:{port}")
        try:
            snap = Snapshot.from_json(await client.get_cluster())
        except EnsembleApiError:
            return None
        return snap.smallest_master_id()

    async def _create(self, host: str, port: int) -> ConfigFlowResult:
        return self.async_create_entry(
            title=f"ensemble ({host})",
            data={CONF_HOST: host, CONF_PORT: port},
        )

    async def async_step_zeroconf(
        self, discovery_info: ZeroconfServiceInfo
    ) -> ConfigFlowResult:
        """Handle mDNS discovery — accept only master adverts (they have HTTP)."""
        props = discovery_info.properties
        role = props.get("role", "")
        if "master" not in role:
            return self.async_abort(reason="not_a_master")

        self._host = discovery_info.host
        # The authoritative HTTP port is in the TXT record; fall back to SRV.
        try:
            self._port = int(props.get("http") or discovery_info.port or DEFAULT_PORT)
        except (TypeError, ValueError):
            self._port = DEFAULT_PORT

        cluster_id = await self._resolve_cluster_id(self._host, self._port)
        if cluster_id is None:
            return self.async_abort(reason="cannot_connect")
        await self.async_set_unique_id(cluster_id)
        self._abort_if_unique_id_configured(
            updates={CONF_HOST: self._host, CONF_PORT: self._port}
        )

        self.context["title_placeholders"] = {"host": self._host}
        return await self.async_step_zeroconf_confirm()

    async def async_step_zeroconf_confirm(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Confirm adding a discovered master."""
        if user_input is not None:
            return await self._create(self._host, self._port)
        return self.async_show_form(
            step_id="zeroconf_confirm",
            description_placeholders={"host": self._host},
        )

    async def async_step_user(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Handle a manual host:port entry."""
        errors: dict[str, str] = {}
        if user_input is not None:
            host = user_input[CONF_HOST]
            port = user_input[CONF_PORT]
            cluster_id = await self._resolve_cluster_id(host, port)
            if cluster_id is None:
                errors["base"] = "cannot_connect"
            else:
                await self.async_set_unique_id(cluster_id)
                self._abort_if_unique_id_configured(
                    updates={CONF_HOST: host, CONF_PORT: port}
                )
                return await self._create(host, port)

        return self.async_show_form(
            step_id="user",
            data_schema=vol.Schema(
                {
                    vol.Required(CONF_HOST): str,
                    vol.Required(CONF_PORT, default=DEFAULT_PORT): int,
                }
            ),
            errors=errors,
        )
