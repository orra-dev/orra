## SDKs v0.2.1

### Features

NO SDK releases.

## SDKs v0.2.0

### Features
- Compensations support.

### Python SDK v0.2.0

#### Features
- Idiomatic developer friendly UX for using compensations.
- Added documentation for using compensations.
- Failed task results are no longer cached.

### Javascript SDK v0.2.0

#### Features
- Idiomatic developer friendly UX for using compensations.
- Added documentation for using compensations.
- Failed task results are no longer cached.

## SDKs v0.1.2

### Features
- Async API based SDK contract specification.
- Language agnostic SDK conformance suite and test harness.

### Python SDK v0.1.2 (**NEW)

#### Features
- New Python SDK that implements the SDK contract specification.
- Published Python SDK to PyPI as orra-sdk - v0.1.2
- Added Python SDK conformance test suite.
- Added documentation for integrating Orra using the Python SDK.
- Echo example showcasing the Python SDK.
- By default, orra-service-key files are prefixed by the service or agent name.

### Javascript SDK v0.1.2

#### Features
- Added JavaScript SDK conformance test suite.
- New developer UX for integrating Services and Agents to Orra.
- Documentation updates.
- Example updates.
- By default, orra-service-key files are prefixed by the service or agent name.

#### Bug fixes
- Ensure service key path directories are created correctly if they don't exist.

## JavaScript SDK v0.1.1

### Features
- Published JS SDK to npm as @orra.dev/sdk - v0.1.1-narwhal

### Bug fixes
- Ensure service key path directories are created correctly if they don't exist.

## JavaScript SDK v0.1.0

### Features

#### Service Management
- Simple registration for services and agents
- Automatic reconnection with exponential backoff (1-30s, max 10 attempts)
- Built-in schema validation for input/output
- Service key persistence with customizable storage
- Automatic versioning and health signaling

#### Task Handling
- Idempotent execution with deduplication and caching
- In-memory result cache (24 hour TTL)
- Stateful execution tracking
- Task lifecycle hooks
- Graceful failure handling

#### Connection Management
- WebSocket connection monitoring and health checks
- Configurable reconnection strategies
- Message deduplication
- Message size limits (10KB)

### Implementation Notes

#### Reliability
- Task deduplication via idempotency keys
- Automatic stale task cleanup
- Health check management
- Connection state tracking

#### Storage
- Configurable service key persistence
- In-memory task caching
- Task execution state management

### Known Limitations
- Single WebSocket connection per service
- No request rate limiting
- In-memory only task cache
- Service recovery limited to connection retries
