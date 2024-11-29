#!/bin/bash
set -e

#
# This Source Code Form is subject to the terms of the Mozilla Public
#  License, v. 2.0. If a copy of the MPL was not distributed with this
#  file, You can obtain one at https://mozilla.org/MPL/2.0/.
#

# Directory containing this script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

echo "Installing dependencies..."
rm -rf "${DIR}/node_modules" "${DIR}/package-lock.json"
npm install --prefix "$DIR" > /dev/null 2>&1

# Run tests
echo "Running JavaScript implementation tests..."
NODE_OPTIONS="--experimental-vm-modules --no-warnings" \
  ORRA_LOGGING=false NODE_ENV="test" npm run test \
  --bail \
  --force-exit \
  "${DIR}"/*.test.js

# Get exit code
EXIT_CODE=$?

# Generate test results
echo "{
  \"implementation\": \"javascript\",
  \"passed\": $([[ $EXIT_CODE == 0 ]] && echo "1" || echo "0"),
  \"total\": 1,
  \"timestamp\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
}" > "${RESULTS_DIR:-../../test-results}/javascript.json"

exit $EXIT_CODE
