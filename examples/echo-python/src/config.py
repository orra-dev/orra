#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import os
from pathlib import Path
from typing import Optional, Dict, Any

def get_persistence_config() -> Dict[str, Any]:
    """Configure service key persistence based on environment"""

    if os.getenv("SVC_ENV") == "development":
        # For local development with Docker, use file persistence with custom path
        default_path = Path.cwd() / ".orra-data" / "echo-service-orra-service-key.json"
        return {
            "persistence_method": "file",
            "persistence_file_path": Path(os.getenv("ORRA_SERVICE_KEY_PATH", default_path))
        }

    # For production environments, use in-memory or other persistence
    async def custom_save(service_id: str) -> None:
        print(f"Service ID saved: {service_id}")

    async def custom_load() -> Optional[str]:
        return None

    return {
        "persistence_method": "custom",
        "custom_save": custom_save,
        "custom_load": custom_load
    }
