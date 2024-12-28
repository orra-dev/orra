#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import time
from typing import Optional, Dict, Any, Callable, Awaitable
from pathlib import Path
import websockets
import asyncio
import json
from datetime import datetime, timezone

import httpx

from .types import PersistenceConfig, T_Input, T_Output
from .persistence import PersistenceManager
from .exceptions import OrraError, ServiceRegistrationError, ConnectionError
from .logger import OrraLogger

MAX_PROCESSED_TASKS_AGE = 24 * 60 * 60  # 24 hours in seconds
MAX_IN_PROGRESS_AGE = 30 * 60  # 30 minutes in seconds
CLEANUP_INTERVAL = 60 * 60  # Run every hour
MAX_MESSAGE_SIZE = 10_485_760 + (1024 * 2)  # 10.5 MB


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

        self.logger = OrraLogger(
            level=log_level,
            enabled=True,
            pretty=log_level.upper() == "DEBUG"
        )
        # Initialize persistence with explicit defaults
        persistence_config = PersistenceConfig(
            method=persistence_method,
            file_path=persistence_file_path,
            custom_save=custom_save,
            custom_load=custom_load
        )
        self._persistence = PersistenceManager(persistence_config)

        # Initialize core state
        self.service_id: Optional[str] = None
        self._url = url.rstrip("/")
        self._api_key = api_key
        self.version: int = 0
        self._ws: Optional[websockets.WebSocketClientProtocol] = None
        self._task_handler: Optional[Callable] = None
        self._message_queue: asyncio.Queue = asyncio.Queue()
        self._pending_messages: Dict[str, Any] = {}
        self._processed_tasks_cache: Dict[str, Any] = {}
        self._in_progress_tasks: Dict[str, Any] = {}
        self._message_id = 0
        self._reconnect_attempts = 0
        self._max_reconnect_attempts = 10
        self._reconnect_interval = 1.0  # 1 second
        self._max_reconnect_interval = 30.0  # 30 seconds
        self._user_initiated_close = False
        self._is_connected = asyncio.Event()

        # Initialize HTTP client for API calls
        self._http = httpx.AsyncClient(
            base_url=self._url,
            headers={"Authorization": f"Bearer {api_key}"},
            timeout=30.0
        )

        self._cleanup_task = asyncio.create_task(self._cleanup_cache_periodically())

    async def register_service_or_agent(
            self,
            name: str,
            description: str,
            input_model: type[T_Input],
            output_model: type[T_Output],
            kind: str
    ) -> None:
        """Register service with control plane"""
        # Load existing service ID if any
        self.service_id = await self._persistence.load_service_id()

        self.logger.debug("Registering service", name=name, existing_service_id=self.service_id)

        try:
            # Convert Pydantic models to JSON schema
            schema = {
                "input": clean_schema(input_model.model_json_schema()),
                "output": clean_schema(output_model.model_json_schema())
            }

            response = await self._http.post(
                url=f"/register/{kind}",
                json={
                    "id": self.service_id,
                    "name": name,
                    "description": description,
                    "schema": schema,
                    "version": self.version
                }
            )
            response.raise_for_status()
            data = response.json()

            # Update service details
            self.service_id = data["id"]
            self.version = data["version"]

            # Update logger with service context
            self.logger.reconfigure(
                service_id=self.service_id,
                service_version=self.version
            )

            # Save service ID
            await self._persistence.save_service_id(self.service_id)

            # Start WebSocket connection
            asyncio.create_task(self._connect_websocket())

        except Exception as e:
            raise ServiceRegistrationError(f"Failed to register service: {e}") from e

    async def _connect_websocket(self) -> None:
        """Establish WebSocket connection"""
        if self._user_initiated_close:
            raise ConnectionError("Cannot connect: SDK is shutting down")

        ws_url = self._url.replace("http", "ws")
        uri = f"{ws_url}/ws?serviceId={self.service_id}&apiKey={self._api_key}"

        try:
            self._ws = await websockets.connect(
                uri,
                max_size=MAX_MESSAGE_SIZE
            )
            self._reconnect_attempts = 0
            self._is_connected.set()
            self.logger.info("WebSocket connection established")

            if not self._message_queue.empty():
                self.logger.info("Resending messages that could not be previously delivered")

            while not self._message_queue.empty():
                message = self._message_queue.get_nowait()
                await self._send_message(message)
                self._message_queue.task_done()

            # Start message processing
            asyncio.create_task(self._process_messages())

        except Exception as e:
            self.logger.error("WebSocket connection failed", error=str(e))
            self._is_connected.clear()
            await self._schedule_reconnect()

    async def _process_messages(self) -> None:
        """Process incoming WebSocket messages"""
        assert self._ws is not None

        try:
            async for message in self._ws:
                try:
                    data = json.loads(message)
                    message_type = data.get("type")

                    if message_type == "ping":
                        await self._handle_ping(data)
                    elif message_type == "ACK":
                        await self._handle_ack(data)
                    elif message_type == "task_request":
                        await self._handle_task(data)
                    else:
                        self.logger.warn(f"Unknown message type: {message_type}")

                except json.JSONDecodeError:
                    self.logger.error("Failed to parse WebSocket message")

        except websockets.ConnectionClosed:
            self._is_connected.clear()
            if not self._user_initiated_close:
                await self._schedule_reconnect()

    async def _schedule_reconnect(self) -> None:
        """Schedule reconnection with exponential backoff"""
        if self._reconnect_attempts >= self._max_reconnect_attempts:
            self.logger.error("Max reconnection attempts reached")
            return

        delay = min(
            self._reconnect_interval * (2 ** self._reconnect_attempts),
            self._max_reconnect_interval
        )
        self._reconnect_attempts += 1

        self.logger.info("Scheduling reconnection", attempt=self._reconnect_attempts, delay_seconds=delay)

        await asyncio.sleep(delay)
        asyncio.create_task(self._connect_websocket())

    async def _handle_task(self, task: dict) -> None:
        """Handle incoming task request"""
        task_id = task.get("id")
        execution_id = task.get("executionId")
        idempotency_key = task.get("idempotencyKey")

        self.logger.debug(
            "Task handling initiated",
            taskId=task_id,
            executionId=execution_id,
            idempotencyKey=idempotency_key,
            handlerPresent=bool(self._task_handler)
        )

        if not self._task_handler:
            self.logger.warn(
                "Received task but no handler is set",
                idempotencyKey=idempotency_key,
                taskId=task_id,
                executionId=execution_id
            )
            return

        # Check cache first
        if cached_result := self._processed_tasks_cache.get(idempotency_key):
            self.logger.debug(
                "Cache hit found",
                taskId=task_id,
                idempotencyKey=idempotency_key,
                resultAge=time.time() - cached_result["timestamp"]
            )
            await self._send_task_result(
                task_id=task_id,
                idempotency_key=idempotency_key,
                execution_id=execution_id,
                result=cached_result["result"],
                error=cached_result.get("error")
            )
            return

        # Check if task is already in progress
        if self._in_progress_tasks.get(idempotency_key):
            self.logger.debug(
                "Task already in progress",
                taskId=task_id,
                idempotencyKey=idempotency_key
            )
            await self._send_task_status(
                task_id=task_id,
                idempotency_key=idempotency_key,
                execution_id=execution_id,
                status="in_progress"
            )
            return

        # Process new task
        start_time = time.time()
        self._in_progress_tasks[idempotency_key] = {"start_time": start_time}

        try:
            # Convert input to Pydantic model if needed
            input_data = task.get("input", {})
            result = await self._task_handler(input_data)

            processing_time = time.time() - start_time
            self.logger.debug(
                "Task processing completed",
                taskId=task_id,
                executionId=execution_id,
                processingTimeMs=processing_time * 1000
            )

            # Cache successful result
            self._processed_tasks_cache[idempotency_key] = {
                "result": result,
                "timestamp": time.time()
            }

            await self._send_task_result(
                task_id=task_id,
                idempotency_key=idempotency_key,
                execution_id=execution_id,
                result=result
            )

        except Exception as e:
            processing_time = time.time() - start_time
            self.logger.error(
                "Task processing failed",
                taskId=task_id,
                executionId=execution_id,
                processingTimeMs=processing_time * 1000,
                error=str(e),
                errorType=type(e).__name__
            )

            # Cache error result
            self._processed_tasks_cache[idempotency_key] = {
                "error": str(e),
                "timestamp": time.time()
            }

            await self._send_task_result(
                task_id=task_id,
                idempotency_key=idempotency_key,
                execution_id=execution_id,
                error=str(e)
            )

        finally:
            del self._in_progress_tasks[idempotency_key]

    async def _send_task_result(
            self,
            task_id: str,
            idempotency_key: str,
            execution_id: str,
            result: Optional[Any] = None,
            error: Optional[str] = None
    ) -> None:
        """Send task execution result"""
        message = {
            "type": "task_result",
            "taskId": task_id,
            "idempotencyKey": idempotency_key,
            "executionId": execution_id,
            "serviceId": self.service_id,
            "result": result,
            "error": error
        }
        await self._send_message(message)

    async def _send_task_status(
            self,
            task_id: str,
            idempotency_key: str,
            execution_id: str,
            status: str
    ) -> None:
        """Send task status update"""
        message = {
            "type": "task_status",
            "taskId": task_id,
            "idempotencyKey": idempotency_key,
            "executionId": execution_id,
            "serviceId": self.service_id,
            "status": status,
            "timestamp": datetime.now(timezone.utc).isoformat()
        }
        await self._send_message(message)

    async def _handle_ping(self, data: dict) -> None:
        """Handle ping message"""
        if data.get("serviceId") != self.service_id:
            self.logger.trace(
                "Received PING for unknown serviceId",
                receivedId=data.get("serviceId")
            )
            return

        self.logger.trace("Received PING")
        await self._send_pong()
        self.logger.trace("Sent PONG")

    async def _send_pong(self) -> None:
        """Send pong response"""
        if self._ws and self._is_connected.is_set():
            message = { "id": "pong", "payload": { "type": 'pong', "serviceId": self.service_id } }
            await self._ws.send(json.dumps(message))


    async def _handle_ack(self, data: dict) -> None:
        """Handle message acknowledgment"""
        message_id = data.get("id")
        self.logger.trace(
            "Received message acknowledgment",
            messageId=message_id
        )
        self._pending_messages.pop(message_id, None)

    async def _send_message(self, message: dict) -> None:
        """Send message with queueing and acknowledgment"""
        message_id = f"msg_{self._message_id}_{message.get('executionId', '')}"
        self._message_id += 1

        wrapped_message = {
            "id": message_id,
            "payload": message
        }

        self.logger.trace(
            "Preparing to send message",
            messageId=message_id,
            messageType=message["type"]
        )

        if not self._is_connected.is_set() or not self._ws:
            self.logger.debug(
                "Connection not ready, queueing message",
                messageId=message_id,
                messageType=message["type"]
            )
            await self._message_queue.put(message)
            return

        try:
            await self._ws.send(json.dumps(wrapped_message))
            self.logger.debug(
                "Message sent successfully",
                messageId=message_id,
                messageType=message["type"]
            )
            self._pending_messages[message_id] = message

            # Schedule timeout for acknowledgment
            asyncio.create_task(self._handle_message_timeout(message_id))

        except Exception as e:
            self.logger.error(
                "Failed to send message, queueing",
                messageId=message_id,
                error=str(e)
            )
            await self._message_queue.put(message)

    async def _handle_message_timeout(self, message_id: str) -> None:
        """Handle message acknowledgment timeout"""
        await asyncio.sleep(5.0)  # 5 second timeout
        if message := self._pending_messages.pop(message_id, None):
            self.logger.debug(
                "Message acknowledgment timeout, re-queueing",
                messageId=message_id
            )
            await self._message_queue.put(message)

    async def _cleanup_cache_periodically(self) -> None:
        """Periodically clean up expired cache entries"""
        while True:
            try:
                if self._user_initiated_close:
                    break

                now = time.time()
                processed_tasks_removed = 0
                in_progress_tasks_removed = 0

                # Cleanup processed tasks cache
                for key, data in list(self._processed_tasks_cache.items()):
                    if now - data["timestamp"] > MAX_PROCESSED_TASKS_AGE:
                        self._processed_tasks_cache.pop(key)
                        processed_tasks_removed += 1

                # Cleanup stale in-progress tasks
                for key, data in list(self._in_progress_tasks.items()):
                    if now - data["start_time"] > MAX_IN_PROGRESS_AGE:
                        self._in_progress_tasks.pop(key)
                        in_progress_tasks_removed += 1

                if processed_tasks_removed or in_progress_tasks_removed:
                    self.logger.debug(
                        "Cache cleanup completed",
                        processedTasksRemoved=processed_tasks_removed,
                        inProgressTasksRemoved=in_progress_tasks_removed,
                        remainingProcessedTasks=len(self._processed_tasks_cache),
                        remainingInProgressTasks=len(self._in_progress_tasks)
                    )

                try:
                    await asyncio.sleep(CLEANUP_INTERVAL)
                except asyncio.CancelledError:
                    break

            except asyncio.CancelledError:
                break
            except Exception as e:
                self.logger.error(
                    "Error during cache cleanup",
                    error=str(e),
                    errorType=type(e).__name__
                )
                await asyncio.sleep(CLEANUP_INTERVAL)

    async def shutdown(self) -> None:
        """Gracefully shutdown the SDK"""
        self.logger.info("Initiating SDK shutdown")
        self._user_initiated_close = True

        # Cancel cleanup task first
        if hasattr(self, '_cleanup_task'):
            self._cleanup_task.cancel()
            try:
                await self._cleanup_task
            except asyncio.CancelledError:
                pass

        # Cancel all remaining tasks
        tasks = []
        for task in asyncio.all_tasks():
            if task is not asyncio.current_task():
                task.cancel()
                tasks.append(task)

        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)

        # Close WebSocket
        if self._ws and self._ws.open:
            self.logger.debug("Closing WebSocket connection")
            await self._ws.close(code=1000, reason="Normal Closure")

        # Close HTTP client
        if hasattr(self, '_http'):
            self.logger.debug("Closing HTTP client")
            await self._http.aclose()

        self.logger.info("SDK shutdown complete")

    async def __aenter__(self) -> 'OrraSDK':
        """Async context manager entry"""
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        """Async context manager exit"""
        await self.shutdown()


def clean_schema(schema: dict) -> dict:
    """Clean schema to remove None types and simplify type definitions"""
    if "properties" not in schema:
        return schema

    for prop_name, prop_schema in schema["properties"].items():
        if "anyOf" in prop_schema:
            # Find the non-null type in anyOf
            types = [t for t in prop_schema["anyOf"] if t.get("type") != "null"]
            if types:
                # Replace anyOf with the single type definition
                prop_schema.update(types[0])
                del prop_schema["anyOf"]

        # Remove default if it exists and is None
        if "default" in prop_schema and prop_schema["default"] is None:
            del prop_schema["default"]

    return schema
