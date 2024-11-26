#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

from enum import Enum
from pathlib import Path
from typing import Optional, Protocol, Callable, Awaitable, TypeVar
from pydantic import BaseModel, Field
from .constants import DEFAULT_SERVICE_KEY_PATH

T_Input = TypeVar('T_Input', bound=BaseModel)
T_Output = TypeVar('T_Output', bound=BaseModel)

# Type alias for handler functions
ServiceHandler = Callable[[T_Input], Awaitable[T_Output]]


class PersistenceMethod(str, Enum):
    FILE = "file"
    CUSTOM = "custom"


class CustomPersistence(Protocol):
    async def save(self, service_id: str) -> None: ...

    async def load(self) -> Optional[str]: ...


class PersistenceConfig(BaseModel):
    method: PersistenceMethod = Field(
        default=PersistenceMethod.FILE,
        description="Method for persisting service identity"
    )
    file_path: Path = Field(
        default=DEFAULT_SERVICE_KEY_PATH,
        description="Path to service key file when using file persistence"
    )
    custom_save: Optional[Callable[[str], Awaitable[None]]] = Field(
        default=None,
        description="Custom save function for service ID persistence"
    )
    custom_load: Optional[Callable[[], Awaitable[Optional[str]]]] = Field(
        default=None,
        description="Custom load function for service ID persistence"
    )

    def model_post_init(self, __context) -> None:
        if self.method == PersistenceMethod.CUSTOM:
            if not (self.custom_save and self.custom_load):
                raise ValueError(
                    "Custom persistence requires both custom_save and custom_load functions"
                )
