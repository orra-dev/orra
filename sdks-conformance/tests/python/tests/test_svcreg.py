#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import os
from pathlib import Path
import pytest
from pydantic import BaseModel
from orra import OrraService, Task
from orra.exceptions import ServiceRegistrationError
from orra.constants import DEFAULT_SERVICE_KEY_DIR
import shutil

TEST_HARNESS_URL = os.getenv("TEST_HARNESS_URL", "http://localhost:8006")

class Input(BaseModel):
    entry: str

class Output(BaseModel):
    entry: str

@pytest.fixture(autouse=True)
async def cleanup():
    yield
    orra_dir = Path.cwd() / DEFAULT_SERVICE_KEY_DIR
    if orra_dir.exists():
        shutil.rmtree(orra_dir)

async def test_minimal_registration(test_harness):
    """Verify basic service registration"""
    project = await test_harness.register_project()

    service = OrraService(
        name="minimal-test-service",
        description="a minimal service",
        url=TEST_HARNESS_URL,
        api_key=project.api_key
    )

    @service.handler()
    async def handle_test(task: Task[Input]) -> Output:
        return Output(entry=task.input.entry)

    await service.start()

    assert service.id.startswith("s_"), "Invalid service ID format"
    assert service.version == 1, "Invalid initial version"

    await service.shutdown()

async def test_persistence(test_harness):
    """Verify service identity persistence"""
    project = await test_harness.register_project()
    saved_service_id = None

    async def custom_save(service_id: str) -> None:
        nonlocal saved_service_id
        saved_service_id = service_id

    async def custom_load() -> str:
        return saved_service_id

    # First service instance
    service_one = OrraService(
        name="persistent-service",
        description="a persistent service",
        url=TEST_HARNESS_URL,
        api_key=project.api_key,
        persistence_method="custom",
        custom_save=custom_save,
        custom_load=custom_load
    )

    @service_one.handler()
    async def handle_test(task: Task[Input]) -> Output:
        return Output(entry=task.input.entry)

    await service_one.start()
    original_id = service_one.id
    await service_one.shutdown()

    # Second service instance
    service_two = OrraService(
        name="persistent-service",
        description="a persistent service",
        url=TEST_HARNESS_URL,
        api_key=project.api_key,
        persistence_method="custom",
        custom_save=custom_save,
        custom_load=custom_load
    )

    @service_two.handler()
    async def handle_test(task: Task[Input]) -> Output:
        return Output(entry=task.input.entry)

    await service_two.start()
    assert service_two.id == original_id
    assert service_two.version == 2  # Version increments on re-registration

    await service_two.shutdown()

async def test_invalid_registration(test_harness):
    """Verify error handling for invalid registrations"""
    await test_harness.register_project()

    # Test invalid API key
    with pytest.raises(ServiceRegistrationError):
        service = OrraService(
            name="invalid-svc",
            description="invalid service",
            url=TEST_HARNESS_URL,
            api_key="sk-orra-invalid-key"
        )
        @service.handler()
        def do_handle(task: Task[Input]) -> Output:
            return Output(entry="value")
        await service.start()

async def test_finality_of_shutdown(test_harness):
    """Verify service cannot be used after shutdown"""
    project = await test_harness.register_project()

    service = OrraService(
        name="closing-service",
        description="a closing service",
        url=TEST_HARNESS_URL,
        api_key=project.api_key
    )

    @service.handler()
    async def handle_test(task: Task[Input]) -> Output:
        return Output(entry=task.input.entry)

    await service.start()
    await service.shutdown()

    with pytest.raises(Exception):
        await service.start()
