# Orra SDK Conformance Suite

## Overview

The Orra SDK Conformance Suite ensures consistent behavior and reliability across different SDK implementations. It verifies that SDKs properly implement critical features like:

- Exactly-once task execution guarantees
- Connection resilience and automatic reconnection
- Message queueing during disconnections
- Health check responses
- Service identity persistence
- Large payload handling

## Why SDK Conformance Matters

When building distributed systems that coordinate multiple services and agents, consistent behavior across all client SDKs is critical. The conformance suite helps maintain this consistency by:

1. **Validating Core Guarantees**: Ensures all SDKs provide the same reliability guarantees that developers depend on
2. **Catching Breaking Changes**: Detects when changes might break SDK compatibility
3. **Enabling Multi-Language Support**: Makes it safer to add new language implementations
4. **Standardizing Behavior**: Creates a consistent developer experience regardless of language choice

## Architecture

The conformance suite consists of:

- **Conformance Server**: Validates message formats and simulates network conditions
- **Test Scenarios**: Standardized test cases defined in `contracts/sdk.yaml`
- **Language-Specific Tests**: Implementations of test scenarios for each SDK
- **Test Runner**: Orchestrates test execution and result collection

## Running the Tests

1. Ensure Docker is running
2. Execute the test runner:
```bash
./scripts/run-tests.sh
```

The script will:
- Start the SDK test harness
- Run tests for all SDK implementations
- Generate a test report at `test-results/report.json`

## Test Results

Results are generated in JSON format with details about:
- Test execution status
- Coverage of required behaviors
- Performance metrics
- Protocol compliance

Example:
```json
{
  "implementation": "javascript",
  "passed": 1,
  "total": 1,
  "timestamp": "2024-11-22T10:00:00Z"
}
```

## Adding New SDK Tests

Follow these steps to add tests for a new SDK implementation:

1. Create a new directory under `tests/` for your language
2. Implement the test scenarios defined in `contracts/sdk.yaml`
3. Create a `run.sh` script that executes your tests
4. Generate results in the specified JSON format

See the `tests/javascript` directory for a reference implementation.
