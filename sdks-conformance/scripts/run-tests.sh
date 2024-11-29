#!/bin/bash

#
# This Source Code Form is subject to the terms of the Mozilla Public
#  License, v. 2.0. If a copy of the MPL was not distributed with this
#  file, You can obtain one at https://mozilla.org/MPL/2.0/.
#

# Directory where this script lives
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REBUILD=${REBUILD:-true}
TEST_IMPL=${TEST_IMPL:-"all"}
WEBHOOK_URL=${WEBHOOK_URL:-http://sdk-test-harness:8006/webhook-test}

# Check if Docker daemon is running
if (! docker stats --no-stream &> /dev/null); then
    echo "Error: Docker daemon is not running"
    exit 1
fi

echo "🪡🪡🪡 Running the Orra SDK Conformance Suite..."
echo ""

echo "Starting test environment..."
if [ "$REBUILD" ==   "true" ]; then
    docker compose -f "${PROJECT_ROOT}/test-harness/docker-compose.yaml" up --build -d --quiet-pull > /dev/null 2>&1
else
    docker compose -f "${PROJECT_ROOT}/test-harness/docker-compose.yaml" up -d --quiet-pull > /dev/null 2>&1
fi

# Function to cleanup on exit
cleanup() {
  echo "Cleaning up test environment..."
  docker compose -f "${PROJECT_ROOT}/test-harness/docker-compose.yaml" down > /dev/null 2>&1
}
trap cleanup EXIT

# Run implementation tests
echo "Running implementation tests..."
FAILED=0

# Track test results
RESULTS_DIR="${PROJECT_ROOT}/test-results"
mkdir -p "${RESULTS_DIR}"
rm -rf "${RESULTS_DIR}/*.json"

# Run tests for each implementation
for IMPL_DIR in "${PROJECT_ROOT}"/tests/*/; do
  if [ -d "$IMPL_DIR" ] && [ "$(basename "$IMPL_DIR")" != "shared" ]; then
    IMPL_NAME=$(basename "$IMPL_DIR")

    if [ "$TEST_IMPL" != "all" ] && [ "$TEST_IMPL" != "$IMPL_NAME" ]; then
      continue
    fi

    echo "Testing $IMPL_NAME implementation..."

    # Run implementation-specific test script
    if [ -f "$IMPL_DIR/run.sh" ]; then
      if ! (cd "$IMPL_DIR" && WEBHOOK_URL="$WEBHOOK_URL" bash "$IMPL_DIR"/run.sh); then
        FAILED=1
        echo "❌ $IMPL_NAME tests failed"
      else
        echo "✅ $IMPL_NAME tests passed"
      fi
    fi
  fi
done

# Generate test report
if ls "${RESULTS_DIR}"/*.json 1> /dev/null 2>&1; then
  echo ""
  echo "Generating test report..."
  jq -s '.' "${RESULTS_DIR}"/*.json > "${RESULTS_DIR}/report.json"
else
  echo "No JSON test result files found in ${RESULTS_DIR}"
#  exit 1
fi

# Output summary
if [ -f "${RESULTS_DIR}/report.json" ]; then
  echo "Test Summary:"
  echo "============"
  jq -r '.[] | "\(.implementation): \(.passed)/\(.total) tests passed"' "${RESULTS_DIR}/report.json"
fi

exit $FAILED
