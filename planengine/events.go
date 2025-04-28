/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

// Common event names for telemetry tracking to ensure consistency
const (
	// Server events
	EventServerStart = "server_start"
	EventServerStop  = "server_stop"

	// Project events
	EventProjectCreated = "project_created"
	EventProjectUpdated = "project_updated"

	// Service events
	EventServiceRegistered = "service_registered"
	EventServiceUpdated    = "service_updated"

	// Execution plan events
	EventExecutionPlanAttempted = "orchestration_created"
	EventExecutionPlanCompleted = "orchestration_completed"
	EventExecutionPlanFailed    = "orchestration_failed"

	// Domain grounding events
	EventGroundingApplied = "grounding_applied"
	EventGroundingRemoved = "grounding_removed"

	// Compensation events
	EventCompensationStarted   = "compensation_started"
	EventCompensationCompleted = "compensation_completed"
	EventCompensationFailed    = "compensation_failed"
)
