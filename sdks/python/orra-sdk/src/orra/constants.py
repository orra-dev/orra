#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

from pathlib import Path

DEFAULT_SERVICE_KEY_DIR = ".orra-data"
DEFAULT_SERVICE_KEY_FILE = "orra-service-key.json"
DEFAULT_SERVICE_KEY_PATH = Path.cwd() / DEFAULT_SERVICE_KEY_DIR / DEFAULT_SERVICE_KEY_FILE
