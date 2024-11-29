## Control Plane v0.1.2

### Features
- New healthcheck endpoint added to the control plane
- Dockerfile updates.
- Docker compose file updates.
- Added schema validations to a Service.
-
### Bug fixes
- Corrected the Project webhooks json tag.
- Ensured the version for registered service or agent is sent back the client.
- Update service and agent names per the new rules.

## Control Plane v0.1.1

### Features
- Documentation updates for all examples.
- All examples now run using Docker.

## Control Plane v0.1.0

### Features

#### Dynamic Orchestration
- LLM-powered decomposition into parallel execution plans
- Vector cache for efficient plan reuse with parameterization
- Intelligent service discovery and capability matching
- Support for services and agents with schema validation

#### Reliability
- Exactly-once execution through idempotency store (24-hour retention)
- Smart service health handling:
    - WebSocket heartbeat monitoring
    - Automatic pause on outages (up to 30-minute recovery)
    - Short-term retries with exponential backoff (up to 5 attempts)
- Retryable vs permanent failure classification

#### State Management
- Append-only log for execution history
- Real-time task status tracking
- Support for parallel execution
- Result aggregation with ordering guarantees

#### Integration
- HTTP webhook delivery for results
- Multiple API key management
- Structured service schema validation

### Implementation Notes
- Lease-based idempotency tracking
- Vector-based semantic caching for LLM plans
- Task dependency resolution
- Resource cleanup for completed orchestrations

### Known Limitations
- HTTP-only webhook delivery
- Manual service discovery/registration
- Single WebSocket connection per service
- Non-configurable task timeouts
