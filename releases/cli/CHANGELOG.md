## CLI v0.2.5

### Features
- Added new `comp-fail` command to manage and monitor compensation failures
  - `comp-fail ls` - List failed compensations with filtering options
  - `comp-fail inspect` - Show detailed information about a compensation failure
  - `comp-fail resolve` - Mark a compensation failure as resolved
  - `comp-fail ignore` - Mark a compensation failure as ignored
  - `comp-fail webhooks` - Manage compensation failure notification webhooks

### Improvements
- Enhanced task monitoring with abort feature
- Added compensation context support for better failure tracking
- Improved orchestration and compensation failures monitoring

## CLI v0.2.4

### Features
- Increased CLI timeout to support using local LLMs for Plan Engine

# Documentation
- Added CLI reasoning traces example to documentation for better debugging

## CLI v0.2.3

### Features
- View task progress updates using the inspect command's new `--updates` and `--long-updates` flags.
- Updated the CLI docs to highlight the latest inspect flags.

## CLI v0.2.2

### Features
- Migrated towards a Plan Engine away from a Control Plane.
- Minor bug fixes.

## CLI v0.2.1

### Features
- Grounding support added using `orra grounding` command.
- Various bug fixes.

## CLI v0.2.0

### Features
- New `--health-check-grace-period` / `-g` flag to set timeout durations for orchestration tasks.
- Compensations support for the `orra ps` command.
- Compensations support for the `orra inspect` command.

## CLI v0.1.3

### Features
- New `--timeout` / `-t` flag to set timeout durations for orchestration tasks.

## CLI v0.1.2

### Bug fixes
- Minor fixes and cleanup.

## CLI v0.1.1

### Bug fixes
- Inspect CLI command should explicitly notify users of an unknown orchestration.

## CLI v0.1.0

### Features
- Initial release of the CLI tool
- Support for project management (`orra projects`)
- Support for webhook configuration (`orra webhooks`)
- Support for API key management (`orra api-keys`)
- Support for orchestrated actions listing (`orra ps`)
- Support for detailed inspection of an orchestrated action (`orra inspect`)
- Support for running orchestrations directly from the CLI to verify your orra setup (`orra verify`)

### Implementation Notes
- Implements local configuration management
- Supports multiple projects and contexts

### Known Limitations
- Config has to be reset when the control plane is restarted (`orra config reset`)
