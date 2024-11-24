#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import sys
from typing import Optional, Any
import structlog
from structlog.types import Processor, EventDict, BindableLogger
from structlog.processors import TimeStamper

class OrraLogger:
    def __init__(
            self,
            level: str = "INFO",
            *,
            service_id: Optional[str] = None,
            service_version: Optional[int] = None,
            enabled: bool = True,
            pretty: bool = False
    ):
        """Initialize the Orra logger

        Args:
            level: Log level (TRACE, DEBUG, INFO, WARNING, ERROR, CRITICAL)
            service_id: Service ID for context
            service_version: Service version for context
            enabled: Whether logging is enabled
            pretty: Whether to use pretty printing for development
        """
        self.enabled = enabled
        if not self.enabled:
            # No-op logger when disabled
            self.logger = structlog.get_logger("orra").bind(enabled=False)
            return

        processors_list: list[Processor] = [
            structlog.stdlib.add_log_level,
            structlog.stdlib.add_log_level_number,
            TimeStamper(fmt="iso", utc=True),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            self._add_base_context,
        ]

        if pretty:
            # Development-friendly console output
            processors_list.append(structlog.dev.ConsoleRenderer())
        else:
            # Production JSON formatting
            processors_list.append(structlog.processors.JSONRenderer())

        structlog.configure(
            processors=processors_list,
            context_class=dict,
            logger_factory=structlog.PrintLoggerFactory(file=sys.stderr),
            wrapper_class=BindableLogger,  # Type is correct here
            cache_logger_on_first_use=True,
        )

        self.logger = structlog.get_logger("orra")
        self._base_context = {
            "sdk": "orra-python",
            "service_id": service_id,
            "service_version": service_version
        }

    def _add_base_context(
            self,
            logger: Any,
            name: str,
            event_dict: EventDict
    ) -> EventDict:
        """Add base context to all log events"""
        for key, value in self._base_context.items():
            if value is not None:
                event_dict[key] = value
        return event_dict

    def reconfigure(
            self,
            service_id: Optional[str] = None,
            service_version: Optional[int] = None
    ) -> None:
        """Update logger configuration with new service details"""
        if service_id is not None:
            self._base_context["service_id"] = service_id
        if service_version is not None:
            self._base_context["service_version"] = service_version

    def _should_log(self) -> bool:
        """Check if logging is enabled"""
        return self.enabled

    def error(self, msg: str, **kwargs: Any) -> None:
        """Log an error message"""
        if self._should_log():
            self.logger.error(msg, **kwargs)

    def warn(self, msg: str, **kwargs: Any) -> None:
        """Log a warning message"""
        if self._should_log():
            self.logger.warn(msg, **kwargs)

    def info(self, msg: str, **kwargs: Any) -> None:
        """Log an info message"""
        if self._should_log():
            self.logger.info(msg, **kwargs)

    def debug(self, msg: str, **kwargs: Any) -> None:
        """Log a debug message"""
        if self._should_log():
            self.logger.debug(msg, **kwargs)

    def trace(self, msg: str, **kwargs: Any) -> None:
        """Log a trace message"""
        if self._should_log():
            self.logger.debug(msg, **kwargs)  # structlog doesn't have trace, map to debug
