#!/bin/bash

#
# This Source Code Form is subject to the terms of the Mozilla Public
#  License, v. 2.0. If a copy of the MPL was not distributed with this
#  file, You can obtain one at https://mozilla.org/MPL/2.0/.
#

# Directory where this script lives
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Ensure control plane image exists
if [[ "$(docker images -q orra-control-plane-dev 2> /dev/null)" == "" ]]; then
  echo "Building control plane image..."
  docker build -t orra-control-plane-dev -f controlplane/Dockerfile .
fi

# Start services
echo "Starting test environment..."
docker compose -f "${PROJECT_ROOT}/proxy/docker-compose.yml" up -d

# Function to cleanup on exit
cleanup() {
  echo "Cleaning up test environment..."
  docker compose -f "${PROJECT_ROOT}/proxy/docker-compose.yml" down
}
trap cleanup EXIT

# Wait for proxy to be ready
echo "Waiting for services to be ready..."
for i in {1..30}; do
  if curl -s http://localhost:8006/health > /dev/null; then
    break
  fi
  sleep 1
  if [ $i -eq 30 ]; then
    echo "Timeout waiting for proxy to start"
    exit 1
  fi
done

# Run implementation tests
echo "Running implementation tests..."
FAILED=0

# Track test results
RESULTS_DIR="${PROJECT_ROOT}/test-results"
mkdir -p "${RESULTS_DIR}"

# Run tests for each implementation
for IMPL_DIR in ${PROJECT_ROOT}/tests/*/; do
  if [ -d "$IMPL_DIR" ] && [ "$(basename "$IMPL_DIR")" != "shared" ]; then
    IMPL_NAME=$(basename "$IMPL_DIR")
    echo "Testing $IMPL_NAME implementation..."

    # Run implementation-specific test script
    if [ -f "$IMPL_DIR/run.sh" ]; then
      if ! bash "$IMPL_DIR/run.sh"; then
        FAILED=1
        echo "❌ $IMPL_NAME tests failed"
      else
        echo "✅ $IMPL_NAME tests passed"
      fi
    fi
  fi
done

# Generate test report
echo "Generating test report..."
jq -s '.' ${RESULTS_DIR}/*.json > ${RESULTS_DIR}/report.json

# Output summary
echo "Test Summary:"
echo "============"
jq -r '.[] | "\(.implementation): \(.passed)/\(.total) tests passed"' ${RESULTS_DIR}/report.json

exit $FAILED
