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
		return FailedNotRetryable, err
	}

	// Check if each original parameter exists in TaskZero's input
	var missingParams []string
	for _, param := range params {
		_, found := taskZero.Input[param.Field]
		if !found {
			missingParams = append(missingParams, param.Field)
		}
	}

	if len(missingParams) > 0 {
		// Get targeted services for the third retry error message
		var serviceIDs []string
		if retryCount >= 2 {
			serviceMap := make(map[string]struct{})
			for _, task := range plan.Tasks {
				if strings.EqualFold(task.ID, TaskZero) {
					continue
				}
				serviceMap[task.Service] = struct{}{}
			}

			for serviceID := range serviceMap {
				serviceIDs = append(serviceIDs, serviceID)
			}
		}

		// Generate the appropriate error message based on retry count
		switch retryCount {
		case 0:
			// First attempt - simple error
			return Failed, fmt.Errorf("action parameters missing from TaskZero: %v", missingParams)

		case 1:
			// First retry - more detailed explanation
			return Failed, fmt.Errorf("action parameters missing from TaskZero: %v. Action parameters should only be added to TaskZero and never have their values embedded into another parameter. This may be caused because no downstream task has a service that accepts those action params as inputs, which is acceptable", missingParams)

		default:
			// Final retry - suggest service schema changes
			return Failed, fmt.Errorf("action parameters missing from TaskZero: %v. A downstream task may require the missing action params as part of their service's input schema - these are the targeted services: %v", missingParams, serviceIDs)
		}
	}

	return Continue, nil
}
