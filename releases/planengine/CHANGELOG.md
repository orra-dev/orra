## Plan Engine v0.2.4

### Breaking Changes
Simplified LLM and embeddings configuration to center around supported models rather than providers
- Changed environment variable scheme from `REASONING_PROVIDER`/`REASONING_MODEL`/`REASONING_API_KEY` and `PLAN_CACHE_OPENAI_API_KEY` to a more cohesive approach with:
  - `LLM_MODEL`/`LLM_API_KEY`/`LLM_API_BASE_URL` for reasoning models
  - `EMBEDDINGS_MODEL`/`EMBEDDINGS_API_KEY`/`EMBEDDINGS_API_BASE_URL` for embedding models
- Dropped support for `deepseek-r1-distill-llama-70b` model
- Added support for self-hosted models with new configuration structure in `_env` template

### Features
- Added support for running the Plan Engine using self-hosted LLMs and embedding models
  - Added support for `deepseek-r1` and `qwq-32b` as reasoning models
  - Added support for `jina-embeddings-v2-small-en` as embedding model
  - All model endpoints must be OpenAI API-compatible
- Improved plan validation with better action parameters handling
  - Added validation to ensure all action parameters are properly included in Task Zero
  - Added validation against composite references in generated plans

### Bug fixes
- Fixed ordering of generated PDDL domain action parameters to prevent non-deterministic map key iteration
- Fixed issue where execution plan non-task referenced inputs were not properly added to task zero
- Increased grounding threshold to make grounding work at an acceptable level
- Updated prompt to clarify expected task input pattern and required JSON markers in output

### Documentation
- Added extensive documentation on model configuration and self-hosting options
- Added comprehensive guide for model configuration and self-hosting
- Fixed typos and improved wording throughout documentation
- Project README: Added new guide: "From Fragile to Production-Ready Multi-Agent App (with Cloudflare Agents)"
- Project README: Updated with improved instructions for self-hosting and on-premises deployment
- Project README: Removed "alpha" labels and updated coming soon sections

## Plan Engine v0.2.3

### Features
- Can accept task interim results as progress updates from coordinated tasks.
- Store task interim results in an orchestration's log.
- Created a new message payload type `task_interim_result` dedicated to processing progress updates.
- Bumped version to v0.2.3

### Documentation
- Updated the core features documentation with an explanation for task interim results .

## Plan Engine v0.2.2

### Features
- Migrated towards a Plan Engine away from a Control Plane.

### Documentation
- General README updates across the project.
- General documentation updates and consolidation.

## Control Plane v0.2.1

### Features
- Execution plan generation using Reasoning models.
- Added domain grounding.
- Execution plan validation based pn grounding capability matching
- Further execution plan validation using PDDL
- BadgerDB for persisting control plane state.

### Bug fixes
- Various bug fixes.

### Documentation
- General README updates across the project.
- Grounding documentation.
- Grounding template.

## Control Plane v0.2.0

### Features
- Implemented comprehensive and robust Compensation handling system. 🎉
- Improved and tidied up service health monitoring and pause mechanisms in task workers. 😅
- Added control plane version headers for all responses.
- Enhanced service and agent schemas to accept arrays as properties.
- Enhanced error reporting for unhealthy services.

### Bug fixes
- Fixed various compensation worker hidden bugs.
- Fixed various task worker obscure bugs.

### Documentation
- General README updates across the project.
- Better explanation for what is considered an Agent in Orra.
- Added documentation for Compensations.

### Dev support
- The `update_examples.sh` now supports crewai-ghost-writers example.

## Control Plane v0.1.3

### Features
- Orchestration task timeout can be configured and defaults to 30 seconds.

### Bug fixes
- Corrected OpenAI API ENV VAR name in control plane.
- Update examples script now removes service files ending with "-orra-service-key.json".
- Improved echo example poetry package file.
- Ignored the orra-data directory content but keep the directory in the echo python example.
- Updated tool call id schema to latest schema in example Delivery Agent to fix Mistral API failures.

### Documentation
- General README fixes across the project.
- Explained that Orra is Agent framework and language agnostic in project README.
- Fixed "setting up control plane" broken link in example's README.
- Use single quotes in CLI `verify run` command example.
- Showcase how to set up and run agent plus service prototyping in a single file.

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
