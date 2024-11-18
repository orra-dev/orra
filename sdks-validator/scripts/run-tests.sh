#!/bin/bash

#
# This Source Code Form is subject to the terms of the Mozilla Public
#  License, v. 2.0. If a copy of the MPL was not distributed with this
#  file, You can obtain one at https://mozilla.org/MPL/2.0/.
#

# Directory where this script lives
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTROL_PLANE_ROOT="$(cd "${SCRIPT_DIR}/../../controlplane" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REBUILD=${REBUILD:-true}

echo "CONTROL_PLANE_ROOT: ${CONTROL_PLANE_ROOT}"
echo "PROJECT_ROOT: ${PROJECT_ROOT}"

# Check if Docker daemon is running
if (! docker stats --no-stream &> /dev/null); then
    echo "Error: Docker daemon is not running"
    exit 1
fi

# Start services
echo "Starting test environment..."
if [ "$REBUILD" = "true" ]; then
    docker compose -f "${PROJECT_ROOT}/proxy/docker-compose.yaml" up --build -d
else
    docker compose -f "${PROJECT_ROOT}/proxy/docker-compose.yaml" up -d
fi

# Function to cleanup on exit
cleanup() {
  echo "Cleaning up test environment..."
  docker compose -f "${PROJECT_ROOT}/proxy/docker-compose.yaml" down
}
trap cleanup EXIT

# Run implementation tests
echo "Running implementation tests..."
FAILED=0

# Track test results
RESULTS_DIR="${PROJECT_ROOT}/test-results"
mkdir -p "${RESULTS_DIR}"

# Run tests for each implementation
for IMPL_DIR in "${PROJECT_ROOT}"/tests/*/; do
  if [ -d "$IMPL_DIR" ] && [ "$(basename "$IMPL_DIR")" != "shared" ]; then
    IMPL_NAME=$(basename "$IMPL_DIR")
    echo "Testing $IMPL_NAME implementation..."

    if [ -f "$IMPL_DIR/package.json" ]; then
      echo "Running npm tests for $IMPL_NAME..."

      # Run implementation-specific test script
      if [ -f "$IMPL_DIR/run.sh" ]; then
        if ! (cd "$IMPL_DIR" && bash "$IMPL_DIR"/run.sh); then
          FAILED=1
          echo "❌ $IMPL_NAME tests failed"
        else
          echo "✅ $IMPL_NAME tests passed"
        fi
      fi
    fi
  fi
done

# Generate test report
echo "Generating test report..."
jq -s '.' "${RESULTS_DIR}/*.json" > "${RESULTS_DIR}/report.json"

# Output summary
echo "Test Summary:"
echo "============"
jq -r '.[] | "\(.implementation): \(.passed)/\(.total) tests passed"' "${RESULTS_DIR}/report.json"

exit $FAILED
