#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import os
from pydantic import BaseModel
from orra import OrraSDK

class EchoInput(BaseModel):
    message: str

class EchoOutput(BaseModel):
    message: str

async def test_echo_service(test_harness):
    """Verify basic service registration and task execution"""
    project = await test_harness.register_project()

    orra = OrraSDK(
        url=os.getenv("ORRA_URL", "http://localhost:8005"),
        api_key=project.api_key
    )

    @orra.service(
        name="echo-service",
        description="Echo test service",
        input_model=EchoInput,
        output_model=EchoOutput
    )
    async def handle_echo(task_input: EchoInput) -> EchoOutput:
        return EchoOutput(message=task_input.message)

    await orra.run()

    result = await test_harness.run_conformance_test(
        service_id=orra.service_id,
        test_id="echo"
    )

    assert result["status"] == "completed"
    assert len(result["results"]) > 0
    task_result = next(r for r in result["results"] if r["type"] == "task_result")
    assert task_result["result"]["message"] == "Hello World"
