#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import os
import base64
from pathlib import Path
import pytest
from pydantic import BaseModel
from orra import OrraService, Task
from orra.constants import DEFAULT_SERVICE_KEY_DIR
import shutil

TEST_HARNESS_URL = os.getenv("TEST_HARNESS_URL", "http://localhost:8006")
MAX_MESSAGE_SIZE = 10 * 1024 * 1024  # 10MB limit

class Input(BaseModel):
    message: str
    size: int

class Output(BaseModel):
    validatedSize: int  # Match JS camelCase
    checksum: str

@pytest.fixture(autouse=True)
async def cleanup():
    yield
    orra_dir = Path.cwd() / DEFAULT_SERVICE_KEY_DIR
    if orra_dir.exists():
        shutil.rmtree(orra_dir)

async def test_large_payload(test_harness):
    """Verify large payload handling capability"""
    project = await test_harness.register_project()

    service = OrraService(
        name="large-payload-service",
        description="Service for testing large payload handling",
        url=TEST_HARNESS_URL,
        api_key=project.api_key
    )

    @service.handler()
    async def handle_large_payload(task: Task[Input]) -> Output:
        print("RECEIVED: handle_large_payload", task.input.size)
        return Output(
            validatedSize=task.input.size,  # Use input size directly
            checksum=base64.b64encode(task.input.message.encode())[:10].decode()
        )

    await service.start()

    result = await test_harness.run_conformance_test(
        api_key=project.api_key,
        service_id=service.id,
        test_id="large_payload",
        poll_timeout=15.0
    )

    assert result["status"] == "completed"
    task_result = next(r for r in result["results"] if r["type"] == "task_result")
    assert task_result["result"]["validatedSize"] == MAX_MESSAGE_SIZE

    await service.shutdown()
