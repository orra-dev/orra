#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import time
import asyncio
import httpx
from typing import Dict, Any
from pydantic import BaseModel, ConfigDict


def to_camel_case(snake_str: str) -> str:
    components = snake_str.split('_')
    return components[0] + ''.join(x.title() for x in components[1:])


class ProjectCredentials(BaseModel):
    id: str
    api_key: str

    model_config = ConfigDict(
        alias_generator=to_camel_case,
        populate_by_name=True
    )


class TestHarness:
    def __init__(self, url: str, webhook_url: str):
        self.url = url.rstrip('/')
        self.webhook_url = webhook_url
        self.http = httpx.AsyncClient(base_url=self.url, timeout=30.0)

    async def register_project(self) -> ProjectCredentials:
        """Register test project and get credentials"""
        response = await self.http.post("/register/project", json={
            "webhooks": [self.webhook_url]
        })
        response.raise_for_status()
        return ProjectCredentials(**response.json())

    async def run_conformance_test(self, api_key:str, service_id: str, test_id: str, poll_timeout: float = 5.0) -> Dict[str, Any]:
        """Run specific conformance test"""
        response = await self.http.post(
            "/conformance-tests",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "serviceId": service_id,
                "testId": test_id
            })
        response.raise_for_status()
        return await self.poll_webhook_result(response.json()["id"], api_key, poll_timeout)

    async def poll_inspect_orchestration(self, result_id: str, api_key: str, timeout: float = 5.0) -> Dict[str, Any]:
        """Poll for webhook test results"""
        end_time = time.time() + timeout
        while time.time() < end_time:
            response = await self.http.get(f"/orchestrations/inspections/{result_id}",
                                           headers={"Authorization": f"Bearer {api_key}"})
            if response.status_code == 200:
                data = response.json()
                if data["status"] in ["completed", "failed"]:
                    return data
            await asyncio.sleep(1)
        raise TimeoutError(f"Test result not available after {timeout}s")

    async def poll_webhook_result(self, result_id: str, api_key: str, timeout: float = 5.0) -> Dict[str, Any]:
        """Poll for webhook test results"""
        end_time = time.time() + timeout
        while time.time() < end_time:
            response = await self.http.get(f"/webhook-test/results/{result_id}",
                                           headers={"Authorization": f"Bearer {api_key}"})
            if response.status_code == 200:
                data = response.json()
                if data["status"] in ["completed", "failed"]:
                    return data
            await asyncio.sleep(1)
        raise TimeoutError(f"Test result not available after {timeout}s")

    async def enable_disconnect(self, api_key: str) -> None:
        """Enable disconnect for next WebSocket connection"""
        await self.http.post("/test-control/enable-disconnect",
                         headers={"Authorization": f"Bearer {api_key}"})

    async def cleanup(self):
        """Cleanup resources"""
        await self.http.aclose()
