#!/bin/bash
#
# This Source Code Form is subject to the terms of the Mozilla Public
#  License, v. 2.0. If a copy of the MPL was not distributed with this
#  file, You can obtain one at https://mozilla.org/MPL/2.0/.
#

set -e
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

echo "Running Python implementation tests..."
poetry run pytest \
  --asyncio-mode=auto \
  --capture=no \
  --exitfirst \
  -v \
  "${DIR}"/tests/

EXIT_CODE=$?

# Generate test results
echo "{
  \"implementation\": \"python\",
  \"passed\": $([[ $EXIT_CODE == 0 ]] && echo "1" || echo "0"),
  \"total\": 1,
  \"timestamp\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
}" > "${RESULTS_DIR:-../../test-results}/python.json"

exit $EXIT_CODE
