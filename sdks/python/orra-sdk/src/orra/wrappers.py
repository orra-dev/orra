#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

from dataclasses import dataclass
from pathlib import Path
from typing import get_type_hints, Callable, Optional, Awaitable, Any, Dict, Generic
from pydantic import ValidationError, BaseModel
from .client import OrraSDK
from .constants import DEFAULT_SERVICE_KEY_DIR, DEFAULT_SERVICE_KEY_FILE
from .exceptions import OrraError
from .types import T_Input, T_Output


@dataclass
class Task(Generic[T_Input]):
    """Wrapper for task inputs"""
    input: T_Input


class OrraBase:
    """Base class for Services and Agents"""

    def __init__(
            self,
            name: str,
            description: str = "",
            url: str = "http://localhost:8005",
            api_key: str = "",
            *,  # Force keyword args for optional params
            persistence_method: str = "file",
            persistence_file_path: Optional[Path] = None,
            custom_save: Optional[Callable[[str], Awaitable[None]]] = None,
            custom_load: Optional[Callable[[], Awaitable[Optional[str]]]] = None,
            log_level: str = "INFO"
    ):
        self._name = name
        self._description = description
        self._handler = None
        self._input_model = None
        self._output_model = None

        # Create core SDK with all options
        targeted_service_key_path = Path.cwd() / DEFAULT_SERVICE_KEY_DIR / f'{self._name}-{DEFAULT_SERVICE_KEY_FILE}'
        self._sdk = OrraSDK(
            url=url,
            api_key=api_key,
            persistence_method=persistence_method,
            persistence_file_path=persistence_file_path or targeted_service_key_path,
            custom_save=custom_save,
            custom_load=custom_load,
            log_level=log_level
        )

    @property
    def id(self) -> Optional[str]:
        return self._sdk.service_id

    @property
    def version(self) -> Optional[int]:
        return self._sdk.version

    def handler(self) -> Callable:
        """Register handler function"""

        def decorator(func: Callable[[Task[T_Input]], Awaitable[T_Output]]):
            hints = get_type_hints(func)
            param_names = list(hints.keys())[:-1]  # Exclude return annotation
            if not param_names:
                raise ValueError("Handler must have one parameter")

            first_param_type = hints[param_names[0]]
            verify_as_task(first_param_type)
            self._input_model = first_param_type.__args__[0]
            self._output_model = hints["return"]

            if not issubclass(self._input_model, BaseModel):
                raise TypeError("Input type must be a Pydantic model")

            if not issubclass(self._output_model, BaseModel):
                raise TypeError("Output type must be a Pydantic model")

            self._handler = func

            # Create internal handler with validation
            async def internal_handler(raw_input: Dict[str, Any]) -> Dict[str, Any]:
                try:
                    self._sdk.logger.debug("Validating input", service=self._name)
                    validated_input = self._input_model.model_validate(raw_input)

                    self._sdk.logger.debug("Executing handler", service=self._name)
                    result = await self._handler(Task(input=validated_input))

                    # Validate output matches schema
                    if not isinstance(result, self._output_model):
                        raise TypeError(f"Handler returned {type(result)}, expected {self._output_model}")

                    return result.model_dump()

                except ValidationError as e:
                    self._sdk.logger.debug(
                        "Input validation failed",
                        service=self._name,
                        errors=e.errors()
                    )
                    raise OrraError(
                        message="Input validation failed",
                        details={
                            "validation_errors": [
                                {
                                    "field": err["loc"][0],
                                    "error": err["msg"],
                                    "type": err["type"]
                                }
                                for err in e.errors()
                            ]
                        }
                    )
                except OrraError:
                    raise
                except Exception as e:
                    self._sdk.logger.error(
                        "Handler error",
                        service=self._name,
                        error=str(e),
                        error_type=type(e).__name__
                    )
                    raise OrraError(
                        message="Service error",
                        details={"error": str(e)}
                    )

            # Set the validated handler on SDK
            self._sdk._task_handler = internal_handler
            return func

        return decorator

    async def start(self):
        """Start processing - handles registration"""
        if not self._handler:
            raise RuntimeError("No handler registered")

        # Registration happens in start
        await self._register()

    async def shutdown(self):
        await self._sdk.shutdown()

    async def _register(self):
        raise NotImplementedError("Must be implemented by subclass")


class OrraService(OrraBase):
    async def _register(self):
        await self._sdk.register_service_or_agent(
            name=self._name,
            description=self._description,
            input_model=self._input_model,
            output_model=self._output_model,
            kind="service"
        )


class OrraAgent(OrraBase):
    async def _register(self):
        await self._sdk.register_service_or_agent(  # Core SDK method for agents
            name=self._name,
            description=self._description,
            input_model=self._input_model,
            output_model=self._output_model,
            kind="agent"
        )

def verify_as_task(first_param_type):
    if not (hasattr(first_param_type, "__origin__") and
            first_param_type.__origin__ is Task):
        raise TypeError("Handler parameter must be Task[YourInputModel]")
