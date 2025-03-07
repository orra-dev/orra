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
	// Maps for different value types
	stringValues := make(map[string]string) // For string values
	jsonValues := make(map[string]string)   // For JSON representations of complex types

	// Process action parameters to build lookup maps
	for _, param := range actionParams {
		field := param.Field

		// For primitive types
		if isPrimitive(param.Value) {
			// Store string representation for primitive types
			stringValues[fmt.Sprintf("%v", param.Value)] = field
		} else {
			// For complex types (arrays, objects), use JSON representation
			jsonBytes, err := json.Marshal(param.Value)
			if err == nil {
				jsonValues[string(jsonBytes)] = field
			}
		}
	}

	var mappings []TaskZeroCacheMapping

	// Find Task0 input values that match action param values
	for task0Field, task0Value := range task0Input {
		matched := false
		actionField := ""
		valueToStore := ""

		// For primitive task0 values, try string match
		if isPrimitive(task0Value) {
			strVal := fmt.Sprintf("%v", task0Value)
			if field, ok := stringValues[strVal]; ok {
				matched = true
				actionField = field
				valueToStore = strVal
			}
		} else {
			// For complex types, try JSON match
			jsonBytes, err := json.Marshal(task0Value)
			if err == nil {
				jsonStr := string(jsonBytes)
				if field, ok := jsonValues[jsonStr]; ok {
					matched = true
					actionField = field
					valueToStore = jsonStr // This is already a valid JSON string
				}
			}
		}

		if matched {
			mappings = append(mappings, TaskZeroCacheMapping{
				Field:       task0Field,
				ActionField: actionField,
				Value:       valueToStore,
			})
		}
	}

	// Sort mappings for consistency
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].Field < mappings[j].Field
	})

	return mappings, nil
}

// isPrimitive checks if a value is a primitive type (string, number, boolean, nil)
func isPrimitive(value interface{}) bool {
	if value == nil {
		return true
	}

	switch value.(type) {
	case string, int, int32, int64, float32, float64, bool, uint, uint64, uint32:
		return true
	default:
		return false
	}
}

func applyParamMappings(
	originalTask0Input map[string]interface{},
	actionParams ActionParams,
	mappings []TaskZeroCacheMapping,
) (map[string]interface{}, error) {
	// Create lookup for new action param values
	actionParamLookup := make(map[string]interface{})
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

		// Always use the new value directly from action params
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
