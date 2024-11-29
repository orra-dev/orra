#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import json
from typing import Optional

from .types import PersistenceConfig, PersistenceMethod
from .exceptions import PersistenceError


class PersistenceManager:
    def __init__(self, config: PersistenceConfig):
        self.config = config

    async def save_service_id(self, service_id: str) -> None:
        """Save service ID using configured persistence method"""
        try:
            if self.config.method == PersistenceMethod.FILE:
                await self._save_to_file(service_id)
            else:
                assert self.config.custom_save  # Validated in PersistenceConfig
                await self.config.custom_save(service_id)
        except Exception as e:
            raise PersistenceError(f"Failed to save service ID: {e}") from e

    async def load_service_id(self) -> Optional[str]:
        """Load service ID using configured persistence method"""
        try:
            if self.config.method == PersistenceMethod.FILE:
                return await self._load_from_file()
            else:
                assert self.config.custom_load  # Validated in PersistenceConfig
                return await self.config.custom_load()
        except Exception as e:
            raise PersistenceError(f"Failed to load service ID: {e}") from e

    async def _save_to_file(self, service_id: str) -> None:
        """Save service ID to file"""
        assert self.config.file_path  # Validated in PersistenceConfig
        path = self.config.file_path

        # Ensure parent directory exists
        path.parent.mkdir(parents=True, exist_ok=True)

        # Write service ID to file atomically
        temp_path = path.with_suffix('.tmp')
        try:
            temp_path.write_text(json.dumps({"id": service_id}))
            temp_path.replace(path)
        finally:
            if temp_path.exists():
                temp_path.unlink()

    async def _load_from_file(self) -> Optional[str]:
        """Load service ID from file"""
        assert self.config.file_path  # Validated in PersistenceConfig
        path = self.config.file_path

        if not path.exists():
            return None

        try:
            data = json.loads(path.read_text())
            return data.get("id")
        except (json.JSONDecodeError, KeyError) as e:
            raise PersistenceError(f"Invalid service key file format: {e}") from e
