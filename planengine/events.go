/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

// Tracked telemetry events

const (
	EventServerStart                   = "server_start"
	EventServerStop                    = "server_stop"
	EventProjectAdded                  = "project_added"
	EventProjectServiceRegistered      = "service_registered"
	EventProjectServiceUpdated         = "service_updated"
	EventExecutionPlanNotActionable    = "execution_plan_not_actionable"
	EventExecutionPlanFailedCreation   = "execution_plan_failed_creation"
	EventExecutionPlanFailedValidation = "execution_plan_failed_validation"
	EventExecutionPlanAttempted        = "execution_plan_attempted"
	EventExecutionPlanCompleted        = "execution_plan_completed"
	EventExecutionPlanAborted          = "execution_plan_aborted"
	EventExecutionPlanFailed           = "execution_plan_failed"
	EventCompensationStarted           = "execution_plan_compensation_started"
	EventCompensationCompleted         = "execution_plan_compensation_completed"
	EventCompensationFailed            = "execution_plan_compensation_failed"
	EventCompensationExpired           = "execution_plan_compensation_expired"
	EventProjectGroundingApplied       = "project_grounding_applied"
	EventProjectGroundingRemoved       = "project_grounding_removed"
)
