/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// validateTaskZeroParams checks if action parameters from the original request were properly added to TaskZero
// and not embedded into other parameters (like the query parameter).
//
// The function compares the original actionParams with what's available in TaskZero inputs to detect
// if any parameters were missed or embedded into another parameter.
//
// The retryCount parameter is used to provide progressively more detailed error messages.
func (p *PlanEngine) validateTaskZeroParams(plan *ExecutionPlan, actionParams json.RawMessage, retryCount int) (Status, error) {
	// Extract TaskZero
	var taskZero *SubTask
	for _, task := range plan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) {
			taskZero = task
			break
		}
	}

	if taskZero == nil {
		return FailedNotRetryable, fmt.Errorf("task zero not found in execution plan")
	}

	// Parse the original action parameters as an array of ActionParam structs
	var params []ActionParam
	if err := json.Unmarshal(actionParams, &params); err != nil {
		// Try as raw map as fallback
		var paramsMap map[string]interface{}
		if err2 := json.Unmarshal(actionParams, &paramsMap); err2 != nil {
			return FailedNotRetryable, fmt.Errorf("failed to unmarshal action parameters: %w", err)
		}

		// Convert map to array of ActionParam
		for field, value := range paramsMap {
			params = append(params, ActionParam{
				Field: field,
				Value: value,
			})
		}
	}

	// Check if each original parameter exists in TaskZero's input
	var missingParams []string
	for _, param := range params {
		_, found := taskZero.Input[param.Field]
		if !found {
			missingParams = append(missingParams, param.Field)
		}
	}

	if len(missingParams) == 0 {
		return Continue, nil
	}

	// Get services details for the error message
	var serviceDetails []string
	for _, task := range plan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) {
			continue
		}
		serviceDetails = append(serviceDetails, task.Service)
	}

	// Format the missing parameters as a comma-separated list
	paramsStr := strings.Join(missingParams, ", ")

	// Generate the appropriate error message based on retry count
	switch retryCount {
	case 0:
		// First attempt - simple error
		return Failed, fmt.Errorf("parameters %s are missing from TaskZero inputs", paramsStr)

	case 1:
		// First retry - more detailed explanation
		return Failed, fmt.Errorf("parameters %s are missing from TaskZero. Your execution plan should include all action parameters explicitly in TaskZero, not embedded inside other parameters. Make sure each parameter is a separate field in TaskZero's input", paramsStr)

	default:
		// Final retry - developer-friendly error with clear guidance
		servicesStr := strings.Join(serviceDetails, ", ")
		return Failed, fmt.Errorf("ORCHESTRATION ERROR: Action parameters [%s] are missing from TaskZero\n\nPROBLEM: Your LLM-generated execution plan is embedding parameters within other fields instead of keeping them as separate parameters.\n\nHOW TO FIX:\n1. Update the service schema for service in [%s] to accept these parameters\n2. OR if these parameters aren't needed, remove them from your orchestration request", paramsStr, servicesStr)
	}
}
