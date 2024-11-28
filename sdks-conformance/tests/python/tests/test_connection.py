#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import os
from pathlib import Path

import pytest
from pydantic import BaseModel
from orra import OrraService, Task
import shutil

from orra.constants import DEFAULT_SERVICE_KEY_DIR

TEST_HARNESS_URL = os.getenv("TEST_HARNESS_URL", "http://localhost:8006")

class Input(BaseModel):
    message: str


class Output(BaseModel):
    message: str
    sequence: int | None = None


@pytest.fixture(autouse=True)
async def cleanup():
    yield
    orra_dir = Path.cwd() / DEFAULT_SERVICE_KEY_DIR
    if orra_dir.exists():
        shutil.rmtree(orra_dir)

async def test_health_check(test_harness):
    """Verify health check functionality"""
    project = await test_harness.register_project()

    service = OrraService(
        name="health-check-service",
        description="Service for testing health checks",
        url=TEST_HARNESS_URL,
        api_key=project.api_key
    )

    @service.handler()
    async def handle_health(task: Task[Input]) -> Output:
        return Output(message=task.input.message)

    await service.start()

    result = await test_harness.run_conformance_test(
        api_key=project.api_key,
        service_id=service.id,
        test_id="health_check",
        poll_timeout=10.0
    )

    assert result["status"] == "completed"
    await service.shutdown()


async def test_reconnection(test_harness):
    """Verify automatic reconnection behavior"""
    project = await test_harness.register_project()

    service = OrraService(
        name="reconnection-service",
        description="Service for testing reconnection",
        url=TEST_HARNESS_URL,
        api_key=project.api_key
    )

    @service.handler()
    async def handle_reconnect(task: Task[Input]) -> Output:
        return Output(message=task.input.message)

    await service.start()

    result = await test_harness.run_conformance_test(
        api_key=project.api_key,
        service_id=service.id,
        test_id="reconnection",
        poll_timeout=35.0
    )

    assert result["status"] == "completed"
    await service.shutdown()


async def test_message_queueing(test_harness):
    """Verify message queueing during disconnection"""
    project = await test_harness.register_project()
    message_count = 0

    service = OrraService(
        name="queue-service",
        description="Service for testing message queueing",
        url=TEST_HARNESS_URL,
        api_key=project.api_key
    )

    @service.handler()
    async def handle_queue(task: Task[Input]) -> Output:
        nonlocal message_count
        message_count += 1
        return Output(
            message=task.input.message,
            sequence=message_count
        )

    await service.start()

    result = await test_harness.run_conformance_test(
        api_key=project.api_key,
        service_id=service.id,
        test_id="message_queueing",
        poll_timeout=10.0
    )

    assert result["status"] == "completed"

    # Verify message order preservation
    sequences = [r["result"]["sequence"] for r in result["results"] if r["type"] == "task_result"]
    assert sequences == sorted(sequences)

    await service.shutdown()
