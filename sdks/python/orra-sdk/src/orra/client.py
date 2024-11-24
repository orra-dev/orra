#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

from typing import Optional, Dict, Any, Callable, Awaitable
from pathlib import Path

import httpx, structlog

from .constants import DEFAULT_SERVICE_KEY_PATH
from .types import PersistenceConfig, ServiceConfig
from .persistence import PersistenceManager
from .exceptions import OrraError

class OrraSDK:
    def __init__(
            self,
            url: str,
            api_key: str,
            *,
            persistence_method: str = "file",
            persistence_file_path: Optional[Path] = None,
            custom_save: Optional[Callable[[str], Awaitable[None]]] = None,
            custom_load: Optional[Callable[[], Awaitable[Optional[str]]]] = None,
            log_level: str = "INFO"
    ):
        """Initialize the Orra SDK client

        Args:
            url: Orra API URL
            api_key: Orra API key
            persistence_method: Either "file" or "custom"
            persistence_file_path: Path to service key file (for file persistence).
                                 Defaults to {cwd}/.orra-data/orra-service-key.json
            custom_save: Custom save function (for custom persistence)
            custom_load: Custom load function (for custom persistence)
            log_level: Logging level
        """
        if not api_key.startswith("sk-orra-"):
            raise OrraError("Invalid API key format")

        # Initialize persistence with explicit defaults
        persistence_config = PersistenceConfig(
            method=persistence_method,
            file_path=persistence_file_path or DEFAULT_SERVICE_KEY_PATH,
            custom_save=custom_save,
            custom_load=custom_load
        )
        self._persistence = PersistenceManager(persistence_config)

        # Initialize core state
        self._url = url.rstrip("/")
        self._api_key = api_key
        self._service_id: Optional[str] = None
        self._version: int = 0
        self._handlers: Dict[str, Any] = {}  # Will be typed properly in next phase

        # Initialize HTTP client for API calls
        self._http = httpx.AsyncClient(
            base_url=self._url,
            headers={"Authorization": f"Bearer {api_key}"},
            timeout=30.0
        )

        # Initialize logger
        self._logger = structlog.get_logger("orra")
