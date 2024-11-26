#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import os
from typing import Optional

from pydantic import BaseModel
from orra import OrraService
from orra.wrappers import Task

TEST_HARNESS_URL = os.getenv("TEST_HARNESS_URL", "http://localhost:8006")
WEBHOOK_URL = os.getenv("WEBHOOK_URL", "http://localhost:8006/webhook-test")


class EchoInput(BaseModel):
    message: str


class EchoOutput(BaseModel):
    message: str


async def test_echo_service(test_harness):
    """Verify basic service registration and task execution"""
    project = await test_harness.register_project()

    async def custom_save(_: str) -> None: return None

    async def custom_load() -> Optional[str]: return None

    echo = OrraService(
        name="echo-service",
        description="Echo test service",
        url=TEST_HARNESS_URL,
        api_key=project.api_key,
        persistence_method="custom",
        custom_save=custom_save,
        custom_load=custom_load
    )

    @echo.handler()
    async def handle_echo(task: Task[EchoInput]) -> EchoOutput:
        return EchoOutput(message=task.input.message)

    await echo.start()

    assert echo.id.startswith("s_"), "Invalid service ID format"
    assert echo.version >= 1, "Invalid service version"

    orchestration_result = await test_harness.http.post(
        "/orchestrations",
        headers={"Authorization": f"Bearer {project.api_key}"},
        json={
            "action": {
                "type": "echo",
                "content": "Echo this message"
            },
            "data": [
                {
                    "field": "message",
                    "value": "Hello World"
                }
            ],
            "webhook": WEBHOOK_URL
        }
    )

    orchestration = orchestration_result.json()
    orchestration_result.raise_for_status()

    result = await test_harness.poll_inspect_orchestration(orchestration["id"], project.api_key)

    assert result["status"] == "completed", "Orchestration failed"
    assert len(result["tasks"]) > 0, "No tasks executed"
    assert result["tasks"][0]["output"] == {"message": "Hello World"}, "Unexpected output"

    await echo.shutdown()
