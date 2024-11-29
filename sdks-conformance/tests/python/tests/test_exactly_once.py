#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import os
from pathlib import Path
import pytest
from pydantic import BaseModel
from orra import OrraService, Task
from orra.constants import DEFAULT_SERVICE_KEY_DIR
import shutil

TEST_HARNESS_URL = os.getenv("TEST_HARNESS_URL", "http://localhost:8006")

class Input(BaseModel):
    message: str

class Output(BaseModel):
    message: str
    count: int

@pytest.fixture(autouse=True)
async def cleanup():
    yield
    orra_dir = Path.cwd() / DEFAULT_SERVICE_KEY_DIR
    if orra_dir.exists():
        shutil.rmtree(orra_dir)

async def test_exactly_once_execution(test_harness):
    """Verify exactly-once execution semantics"""
    project = await test_harness.register_project()
    execution_count = 0

    service = OrraService(
        name="exactly-once-service",
        description="Test service for exactly-once delivery validation",
        url=TEST_HARNESS_URL,
        api_key=project.api_key
    )

    @service.handler()
    async def handle_task(task: Task[Input]) -> Output:
        nonlocal execution_count
        execution_count += 1
        return Output(
            message=task.input.message,
            count=execution_count
        )

    await service.start()

    result = await test_harness.run_conformance_test(
        api_key=project.api_key,
        service_id=service.id,
        test_id="exactly_once",
        poll_timeout=10.0
    )

    assert result["status"] == "completed"

    # Verify task was executed exactly once
    for item in result["results"]:
        assert item["type"] == "task_result"
        assert item["result"]["count"] == 1

    assert execution_count == 1

    await service.shutdown()
