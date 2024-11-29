#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

import os
import pytest
from test_utils import TestHarness

@pytest.fixture
async def test_harness():
    """Provides configured test harness"""
    harness_url = os.getenv("TEST_HARNESS_URL", "http://localhost:8006")
    webhook_url = os.getenv("WEBHOOK_URL", "http://localhost:8006/webhook-test")
    harness = TestHarness(harness_url, webhook_url)
    yield harness
    await harness.cleanup()
