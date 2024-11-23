# SDK Implementation Tests

This directory contains language-specific implementations of the Orra SDK conformance test suite.

## Directory Structure

Each SDK implementation should have its own directory:

```
tests/
├── javascript/     # JavaScript SDK tests
├── python/         # Python SDK tests
├── go/             # Go SDK tests
└── {language}/     # New language implementations
```

## Test Requirements

Each language implementation must test:

1. **Service Registration**
    - Basic registration flow
    - Schema validation
    - Service identity persistence
    - Error handling

2. **Connection Management**
    - Health check handling
    - Reconnection with backoff
    - Message queueing during disconnection

3. **Task Processing**
    - Exactly-once execution
    - Large payload handling
    - Mid-task disconnect recovery

4. **Echo Service**
    - Basic message processing
    - Protocol compliance

## Implementation Guidelines

1. Create a new directory for your language:
```bash
mkdir tests/{language}
```

2. Include required files:
    - Test implementation files
    - Dependencies/package management
    - `run.sh` script for test execution

3. Your `run.sh` should:
    - Set up the test environment
    - Execute all test cases
    - Generate results in `test-results/{language}.json`
    - Clean up after completion

## Test Results Format

Results must be in JSON format:
```json
{
  "implementation": "language-name",
  "passed": "<number>",
  "total": "<number>",
  "timestamp": "ISO-8601-timestamp"
}
```

## Running Individual Implementations

To run tests for a specific implementation:

```bash
# From the sdks-conformance directory
WEBHOOK_URL=http://sdk-test-harness:8006/webhook-test tests/{language}/run.sh
```

## Example Implementation

The `javascript` directory provides a reference implementation showing:
- Test organization
- Error handling
- Results generation
- Clean setup/teardown

Use it as a guide when adding new language implementations.

## Adding New Test Cases

1. Add the test case specification to `contracts/sdk.yaml`
2. Implement the test in all language directories
3. Update documentation to reflect new requirements
4. Verify backwards compatibility
