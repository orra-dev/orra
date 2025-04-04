/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"fmt"
	"regexp"
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
		return FailedNotRetryable, fmt.Errorf("task0 not found in execution plan")
	}

	// Parse the original action parameters as an array of ActionParam structs
	var params []ActionParam
	if err := json.Unmarshal(actionParams, &params); err != nil {
		return FailedNotRetryable, err
	}

	// Check if each original parameter exists in TaskZero's input
	missingParams := findTaskZeroMissingParams(params, taskZero)
	if len(missingParams) == 0 {
		return Continue, nil
	}
	embeddedParams := findEmbeddedActionParams(taskZero, missingParams)
	embeddedInfo := buildEmbeddedParamsInfo(embeddedParams)
	missingParamsStr := strings.Join(missingParams, ", ")

	// Get services details for the error message
	var serviceDetails []string
	for _, task := range plan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) {
			continue
		}
		serviceDetails = append(serviceDetails, task.Service)
	}

	// Generate the appropriate error message based on retry count and embedding status
	switch retryCount {
	case 0:
		// First attempt - simple error
		if len(embeddedInfo) == 0 {
			return Failed, fmt.Errorf("parameters %s are missing from task0 inputs", missingParamsStr)
		}
		return Failed, fmt.Errorf("parameters %s are missing from task0 inputs (embedded in %s)", missingParamsStr, embeddedInfo)

	case 1:
		// First retry - more detailed explanation
		if len(embeddedInfo) == 0 {
			return Failed, fmt.Errorf("parameters %s are missing from task0. Your execution plan should include all action parameters explicitly in task0 as separate fields", missingParamsStr)
		}
		return Failed, fmt.Errorf("parameters %s are missing from task0. They appear to be embedded in %s. Your execution plan should include all action parameters explicitly in task0, not embedded inside other parameters", missingParamsStr, embeddedInfo)

	default:

		servicesStr := strings.Join(serviceDetails, ", ")
		if len(embeddedInfo) == 0 {
			return Failed, fmt.Errorf("ORCHESTRATION ERROR: Parameters [%s] are missing from task0\n\nPROBLEM: The generated execution plan is missing required parameters that were provided in the orchestration request.\n\nHOW TO FIX:\n1. Update the service schema for %s to accept these parameters\n2. OR if these parameters aren't needed, remove them from your orchestration request", missingParamsStr, servicesStr)
		}
		return Failed, fmt.Errorf("ORCHESTRATION ERROR: Parameters [%s] are missing from task0\n\nPROBLEM: The generated execution plan is embedding parameters within other fields (%s) instead of keeping them as separate parameters.\n\nHOW TO FIX:\n1. Update the service schema for %s to accept these parameters\n2. OR if these parameters aren't needed, remove them from your orchestration request", missingParamsStr, embeddedInfo, servicesStr)
	}
}

func findTaskZeroMissingParams(params []ActionParam, taskZero *SubTask) []string {
	var out []string
	for _, param := range params {
		if _, found := taskZero.Input[param.Field]; found {
			continue
		}
		out = append(out, param.Field)
	}
	return out
}

func findEmbeddedActionParams(taskZero *SubTask, missingParams []string) map[string][]string {
	// Check if any missing parameters are embedded in other parameters
	embeddedParams := make(map[string][]string) // Maps parameter name to list of parameters embedded in it

	// Check each parameter in taskZero
	for paramName, paramValue := range taskZero.Input {
		strValue, ok := paramValue.(string)
		if !ok {
			continue
		}

		// Check each missing parameter to see if it's embedded in this value
		for _, missingParam := range missingParams {
			// Create regex patterns to match common embedding patterns
			patterns := []string{
				// Match "param: value" or "param:value"
				fmt.Sprintf(`%s\s*:\s*(\S+)`, regexp.QuoteMeta(missingParam)),
				// Match "param=value"
				fmt.Sprintf(`%s\s*=\s*(\S+)`, regexp.QuoteMeta(missingParam)),
				// Match "param = value"
				fmt.Sprintf(`%s\s*=\s*"([^"]+)"`, regexp.QuoteMeta(missingParam)),
				// Match "param is value" or "param was value"
				fmt.Sprintf(`%s\s+(?:is|was|should be)\s+(\S+)`, regexp.QuoteMeta(missingParam)),
				// Match "with param value" or "has param value"
				fmt.Sprintf(`(?:with|has)\s+%s\s+(\S+)`, regexp.QuoteMeta(missingParam)),
				// Match "param value" or "param value"
				fmt.Sprintf(`\s+%s\s+(\S+)`, regexp.QuoteMeta(missingParam)),
			}

			for _, pattern := range patterns {
				re := regexp.MustCompile(pattern)
				if re.MatchString(strValue) {
					embeddedParams[paramName] = append(embeddedParams[paramName], missingParam)
					break // Found a match for this missing param
				}
			}
		}
	}
	return embeddedParams
}

func buildEmbeddedParamsInfo(embeddedParams map[string][]string) string {
	if len(embeddedParams) == 0 {
		return ""
	}

	var out []string
	for paramName, embedded := range embeddedParams {
		out = append(out, fmt.Sprintf("%s contains [%s]", paramName, strings.Join(embedded, ", ")))
	}
	return strings.Join(out, ", ")
}

// validateNoCompositeTaskZeroRefs checks for multiple task0 references embedded
// within a single input parameter of downstream tasks.
//
// The function returns a Status and error combination similar to other validation
// functions in the codebase. The retryCount parameter is used to provide
// progressively more detailed error messages.
func (p *PlanEngine) validateNoCompositeTaskZeroRefs(plan *ExecutionPlan, retryCount int) (Status, error) {
	// Skip validation if there are no tasks or just task0
	if len(plan.Tasks) <= 1 {
		return Continue, nil
	}

	// Regular expression to find all task0 references
	// Matches $task0.fieldname pattern
	task0RefRegex := regexp.MustCompile(`\$task0\.([a-zA-Z0-9_]+)`)

	// Maps taskID -> paramName -> []refFields
	compositeRefs := make(map[string]map[string][]string)

	// Check each downstream task (skip task0)
	for _, task := range plan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) || task.Input == nil {
			continue
		}

		// For each input parameter, check for composite task0 references
		for paramName, paramValue := range task.Input {
			// We only care about string values that might contain references
			strValue, ok := paramValue.(string)
			if !ok {
				continue
			}

			// Find all task0 references in this parameter value
			matches := task0RefRegex.FindAllStringSubmatch(strValue, -1)
			if len(matches) <= 1 {
				// At most one reference, this is valid
				continue
			}

			// Extract the field names that were referenced
			refFields := make([]string, 0, len(matches))
			for _, match := range matches {
				if len(match) > 1 {
					refFields = append(refFields, "$task0."+match[1])
				}
			}

			// If we found multiple references, record this issue
			if len(refFields) > 1 {
				if _, exists := compositeRefs[task.ID]; !exists {
					compositeRefs[task.ID] = make(map[string][]string)
				}
				compositeRefs[task.ID][paramName] = refFields
			}
		}
	}

	// If no issues found, return success
	if len(compositeRefs) == 0 {
		return Continue, nil
	}

	// Build an error message with progressive detail based on retry count
	switch retryCount {
	case 0:
		// First attempt - simple error
		return Failed, buildCompositeRefSimpleError(compositeRefs)

	case 1:
		// First retry - more detailed explanation
		return Failed, buildCompositeRefDetailedError(compositeRefs)

	default:
		// Final attempt - comprehensive error with actionable guidance
		return Failed, buildCompositeRefFinalError(compositeRefs, plan)
	}
}

// Helper function to build a simple error message for first attempt
func buildCompositeRefSimpleError(compositeRefs map[string]map[string][]string) error {
	var taskIDs []string
	for taskID := range compositeRefs {
		taskIDs = append(taskIDs, taskID)
	}

	return fmt.Errorf("found composite task0 references in tasks: %s", strings.Join(taskIDs, ", "))
}

// Helper function to build a more detailed error for first retry
func buildCompositeRefDetailedError(compositeRefs map[string]map[string][]string) error {
	var sb strings.Builder
	sb.WriteString("found composite task0 references that should be separate parameters.\n")

	for taskID, params := range compositeRefs {
		sb.WriteString(fmt.Sprintf("In task %q:\n", taskID))
		for paramName, refs := range params {
			sb.WriteString(fmt.Sprintf("  Parameter %q contains multiple references: %s\n",
				paramName, strings.Join(refs, ", ")))
		}
	}

	sb.WriteString("\nEach parameter should reference at most one task0 field.")
	return fmt.Errorf(sb.String())
}

// Helper function to build comprehensive error for final retry
func buildCompositeRefFinalError(compositeRefs map[string]map[string][]string, plan *ExecutionPlan) error {
	var sb strings.Builder

	sb.WriteString("ORCHESTRATION ERROR: Found composite task0 references in downstream tasks\n\n")
	sb.WriteString("PROBLEM: The generated execution plan is combining multiple task0 references in single parameters.\n")
	sb.WriteString("Each input parameter should reference at most one task0 field.\n\n")

	sb.WriteString("Details:\n")
	for taskID, params := range compositeRefs {
		sb.WriteString(fmt.Sprintf("- Task %q:\n", taskID))
		for paramName, refs := range params {
			// Get the current value
			var paramValue string
			for _, task := range plan.Tasks {
				if task.ID == taskID {
					if strVal, ok := task.Input[paramName].(string); ok {
						paramValue = strVal
					}
					break
				}
			}

			sb.WriteString(fmt.Sprintf("  • Parameter %q contains multiple references: %s\n",
				paramName, strings.Join(refs, ", ")))
			sb.WriteString(fmt.Sprintf("    Current value: %q\n", paramValue))
			sb.WriteString(fmt.Sprintf("    Expected: only %q instead\n", refs[0]))
		}
	}

	sb.WriteString("\nHOW TO FIX:\n")
	sb.WriteString("If required, update each task's service's input schema for to accept these parameters\n")

	return fmt.Errorf(sb.String())
}
