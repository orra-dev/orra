/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// extractParamMappings discovers which Task0 input fields were derived from action parameters
func extractParamMappings(actionParams ActionParams, task0Input map[string]interface{}) ([]TaskZeroCacheMapping, error) {
	// Create value to field lookup from action params
	valueToField := make(map[string]string)
	for _, param := range actionParams {
		valueToField[param.Value] = param.Field
	}

	var mappings []TaskZeroCacheMapping

	// Find Task0 input values that match action param values
	for task0Field, task0Value := range task0Input {
		strValue, ok := task0Value.(string)
		if !ok {
			continue // Skip non-string values
		}

		// If we find a matching value in action params
		if actionField, exists := valueToField[strValue]; exists {
			mappings = append(mappings, TaskZeroCacheMapping{
				Field:       task0Field,
				ActionField: actionField,
				Value:       strValue,
			})
		}
	}

	// Sort mappings for consistency
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].Field < mappings[j].Field
	})

	return mappings, nil
}

// applyParamMappings creates a new Task0 input using stored mappings and new action params
func applyParamMappings(
	originalTask0Input map[string]interface{},
	actionParams ActionParams,
	mappings []TaskZeroCacheMapping,
) (map[string]interface{}, error) {
	// Create lookup for new action param values
	actionParamLookup := make(map[string]string)
	for _, param := range actionParams {
		actionParamLookup[param.Field] = param.Value
	}

	// Start with a copy of original input
	newTask0Input := make(map[string]interface{})
	for k, v := range originalTask0Input {
		newTask0Input[k] = v
	}

	// Apply mappings to update relevant fields
	for _, mapping := range mappings {
		newValue, exists := actionParamLookup[mapping.ActionField]
		if !exists {
			return nil, fmt.Errorf("missing required action parameter: %s", mapping.ActionField)
		}
		newTask0Input[mapping.Field] = newValue
	}

	return newTask0Input, nil
}

// substituteTask0Params creates a new plan with updated Task0 parameters
func substituteTask0Params(content string, originalInput, newParams json.RawMessage, mappings []TaskZeroCacheMapping) (string, error) {
	// Parse the calling plan
	var plan ExecutionPlan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return "", fmt.Errorf("failed to parse calling plan for task0 param substitution: %w", err)
	}

	// Parse original Task0 input
	var origTask0Input map[string]interface{}
	if err := json.Unmarshal(originalInput, &origTask0Input); err != nil {
		return "", fmt.Errorf("failed to parse original Task0 input: %w", err)
	}

	// Parse new action params
	var actionParams ActionParams
	if err := json.Unmarshal(newParams, &actionParams); err != nil {
		return "", fmt.Errorf("failed to parse new action params: %w", err)
	}

	// Generate new Task0 input using mappings
	newTask0Input, err := applyParamMappings(origTask0Input, actionParams, mappings)
	if err != nil {
		return "", err
	}

	// Find and update Task0 in the plan
	task0Found := false
	for i, task := range plan.Tasks {
		if task.ID == "task0" {
			plan.Tasks[i].Input = newTask0Input
			task0Found = true
			break
		}
	}

	if !task0Found {
		return "", fmt.Errorf("task0 not found in calling plan")
	}

	// Marshal the updated plan
	updatedContent, err := json.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("failed to marshal updated plan: %w", err)
	}

	return string(updatedContent), nil
}
