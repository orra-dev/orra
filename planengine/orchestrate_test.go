/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeService creates a ServiceInfo with the given ID and required input keys.
// It populates the Schema.Input.Properties map so that InputPropKeys returns these keys.
func fakeService(id string, keys []string) *ServiceInfo {
	properties := make(map[string]Spec)
	for _, k := range keys {
		properties[k] = Spec{}
	}
	return &ServiceInfo{
		ID: id,
		Schema: ServiceSchema{
			Input: Spec{
				Properties: properties,
			},
		},
	}
}

// fakeSubTask creates a SubTask with the provided ID, Service ID, and input map.
func fakeSubTask(id, service string, input map[string]any) *SubTask {
	return &SubTask{
		ID:      id,
		Service: service,
		Input:   input,
	}
}

func TestValidateInput(t *testing.T) {
	// Create a dummy PlanEngine to call the method.
	cp := &PlanEngine{}

	testCases := []struct {
		name           string
		services       []*ServiceInfo
		subTasks       []*SubTask
		wantError      bool
		expectMessages []string
	}{
		{
			name: "valid input",
			services: []*ServiceInfo{
				fakeService("svc1", []string{"foo", "bar"}),
			},
			subTasks: []*SubTask{
				fakeSubTask("subtask1", "svc1", map[string]any{
					"foo": "value1",
					"bar": "value2",
				}),
			},
			wantError: false,
		},
		{
			name: "missing required input",
			services: []*ServiceInfo{
				fakeService("svc1", []string{"foo", "bar"}),
			},
			subTasks: []*SubTask{
				fakeSubTask("subtask1", "svc1", map[string]any{
					"foo": "value1",
				}),
			},
			wantError: true,
			expectMessages: []string{
				"service svc1 is missing required input bar in subtask subtask1",
			},
		},
		{
			name: "extra unsupported input",
			services: []*ServiceInfo{
				fakeService("svc1", []string{"foo", "bar"}),
			},
			subTasks: []*SubTask{
				fakeSubTask("subtask1", "svc1", map[string]any{
					"foo": "value1",
					"bar": "value2",
					"baz": "value3",
				}),
			},
			wantError: true,
			expectMessages: []string{
				"input baz not supported by service svc1 for subtask subtask1",
			},
		},
		{
			name: "service not found",
			services: []*ServiceInfo{
				fakeService("svc1", []string{"foo", "bar"}),
			},
			subTasks: []*SubTask{
				fakeSubTask("subtask1", "nonexistent", map[string]any{
					"foo": "value1",
					"bar": "value2",
				}),
			},
			wantError: true,
			expectMessages: []string{
				"service nonexistent not found for subtask subtask1",
			},
		},
		{
			name: "multiple errors",
			services: []*ServiceInfo{
				fakeService("svc1", []string{"foo", "bar"}),
				fakeService("svc2", []string{"alpha", "beta"}),
			},
			subTasks: []*SubTask{
				// For svc1, missing "bar" and with an extra key "extra"
				fakeSubTask("subtask1", "svc1", map[string]any{
					"foo":   "value1",
					"extra": "valueX",
				}),
				// For svc2, missing required "beta"
				fakeSubTask("subtask2", "svc2", map[string]any{
					"alpha": "valueA",
				}),
			},
			wantError: true,
			expectMessages: []string{
				"service svc1 is missing required input bar in subtask subtask1",
				"input extra not supported by service svc1 for subtask subtask1",
				"service svc2 is missing required input beta in subtask subtask2",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			err := cp.validateSubTaskInputs(tc.services, tc.subTasks)
			if tc.wantError {
				assert.Error(t, err)
				// Verify that the error message includes all expected substrings.
				for _, msg := range tc.expectMessages {
					assert.True(t, strings.Contains(err.Error(), msg),
						fmt.Sprintf("expected error to contain %q, but got: %q", msg, err.Error()))
				}
				// Optionally, you can also check that errors.Join was used.
				// If you need to inspect the underlying errors you can use errors.As.
				var joined = err // should wrap a joined error
				assert.Error(t, errors.Unwrap(joined))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeJSONOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		err      string
	}{
		{
			name:     "basic json extraction",
			input:    "```json{\n  \"key\": \"value\"\n}```",
			expected: "{\n  \"key\": \"value\"\n}",
			err:      "",
		},
		{
			name:     "json with surrounding text",
			input:    "Here's some text before\n```json {\n  \"type\": \"object\",\n  \"name\": \"test\"\n} ```\nAnd text after",
			expected: "{\n  \"type\": \"object\",\n  \"name\": \"test\"\n}",
			err:      "",
		},
		{
			name:     "no json markers",
			input:    "Just some regular text",
			expected: "",
			err:      "cannot find opening JSON marker",
		},
		{
			name:     "missing json prefix",
			input:    "``````",
			expected: "``````",
			err:      "cannot find opening JSON marker",
		},
		{
			name:     "has json prefix but no json content",
			input:    "```json```",
			expected: "",
			err:      "cannot parse invalid JSON",
		},
		{
			name:     "missing closing marker",
			input:    "```json\n{\"key\": \"value\"}",
			expected: "",
			err:      "cannot find closing JSON marker",
		},
		{
			name:     "multiple json blocks",
			input:    "```json\n{\"first\": true}```\nSome text\n```json{\"second\": true}```",
			expected: "{\"first\": true}",
		},
		{
			name:     "json with extra whitespace",
			input:    "\n\n  ```json\n{\n    \"key\": \"value\"\n  }\n```  \n\n",
			expected: "{\n    \"key\": \"value\"\n  }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractValidJSONOutput(tt.input)
			if tt.err != "" {
				assert.ErrorContains(t, err, tt.err)
			} else {
				assert.Equal(t, tt.expected, result, "The extracted JSON should match the expected output")
			}
		})
	}
}

func TestCallingPlanMinusTaskZero_DirectValuesMovedToTaskZero(t *testing.T) {
	// Create a test plan engine
	p := &PlanEngine{
		Logger: zerolog.Nop(), // Silent logger for tests
	}

	// Create a plan with TaskZero and task1 with a direct "action" value
	plan := &ExecutionPlan{
		Tasks: []*SubTask{
			{
				ID: TaskZero,
				Input: map[string]any{
					"productId": "laptop-1",
					"userId":    "user-1",
				},
			},
			{
				ID:      "task1",
				Service: "service1",
				Input: map[string]any{
					"action":    "checkAvailability", // Direct value
					"productId": "$task0.productId",  // Already a reference
				},
			},
		},
	}

	// Execute the function
	taskZero, newPlan := p.callingPlanMinusTaskZero(plan)

	// Verify TaskZero now has the "action" field
	assert.Equal(t, "checkAvailability", taskZero.Input["action"], "TaskZero should have action field with value 'checkAvailability'")

	// Verify task1 now references TaskZero for "action"
	task1 := newPlan.Tasks[0]
	assert.Equal(t, "$task0.action", task1.Input["action"], "task1 should reference TaskZero for action")

	// Verify other fields remain unchanged
	assert.Equal(t, "$task0.productId", task1.Input["productId"], "task1.productId should remain as reference to TaskZero")
}

func TestCallingPlanMinusTaskZero_MultipleTasksSameKey(t *testing.T) {
	// Create a test plan engine
	p := &PlanEngine{
		Logger: zerolog.Nop(), // Silent logger for tests
	}

	// Create a plan with TaskZero and two tasks with the same direct key
	plan := &ExecutionPlan{
		Tasks: []*SubTask{
			{
				ID: TaskZero,
				Input: map[string]any{
					"productId": "laptop-1",
				},
			},
			{
				ID:      "task1",
				Service: "service1",
				Input: map[string]any{
					"action":    "checkAvailability", // Direct value
					"productId": "$task0.productId",  // Already a reference
				},
			},
			{
				ID:      "task2",
				Service: "service2",
				Input: map[string]any{
					"action":    "checkAvailability", // Same direct value as task1
					"userId":    "user-1",            // Another direct value
					"productId": "$task0.productId",  // Already a reference
				},
			},
		},
	}

	// Execute the function
	taskZero, newPlan := p.callingPlanMinusTaskZero(plan)

	// Verify TaskZero has both the "action" and "userId" fields
	assert.Equal(t, "checkAvailability", taskZero.Input["action"], "TaskZero should have action field with value 'checkAvailability'")
	assert.Equal(t, "user-1", taskZero.Input["userId"], "TaskZero should have userId field with value 'user-1'")

	// Verify both tasks now reference TaskZero for "action"
	task1 := newPlan.Tasks[0]
	task2 := newPlan.Tasks[1]

	assert.Equal(t, "$task0.action", task1.Input["action"], "task1 should reference TaskZero for action")
	assert.Equal(t, "$task0.action", task2.Input["action"], "task2 should reference TaskZero for action")

	// Verify task2 now references TaskZero for "userId"
	assert.Equal(t, "$task0.userId", task2.Input["userId"], "task2 should reference TaskZero for userId")
}

func TestCallingPlanMinusTaskZero_ComplexExecutionPlanExample(t *testing.T) {
	// Create a test plan engine
	p := &PlanEngine{
		Logger: zerolog.Nop(), // Silent logger for tests
	}

	// Create a plan based on the example in the question
	planJSON := `{
		"tasks": [
			{
				"id": "task0",
				"input": {
					"productId": "laptop-1",
					"userId": "user-1"
				}
			},
			{
				"id": "task1",
				"service": "s_bsygtgcnsszpijinlxorkfgqmnfb",
				"input": {
					"action": "checkAvailability",
					"productId": "$task0.productId"
				}
			},
			{
				"id": "task2",
				"service": "s_avmoeyyudffierttltpsdjstpjxe",
				"input": {
					"inStock": "$task1.inStock",
					"productId": "$task0.productId",
					"userId": "$task0.userId"
				}
			}
		],
		"parallel_groups": [
			["task1"],
			["task2"]
		]
	}`

	var plan ExecutionPlan
	err := json.Unmarshal([]byte(planJSON), &plan)
	require.NoError(t, err, "Failed to unmarshal test plan")

	// Execute the function
	taskZero, newPlan := p.callingPlanMinusTaskZero(&plan)

	// Verify TaskZero now has the "action" field
	assert.Equal(t, "checkAvailability", taskZero.Input["action"], "TaskZero should have action field with value 'checkAvailability'")

	// Find task1 in the new plan
	var task1 *SubTask
	for _, task := range newPlan.Tasks {
		if task.ID == "task1" {
			task1 = task
			break
		}
	}

	require.NotNil(t, task1, "task1 not found in the new plan")

	// Verify task1 now references TaskZero for "action"
	assert.Equal(t, "$task0.action", task1.Input["action"], "task1 should reference TaskZero for action")

	// Verify parallel groups are maintained
	expectedGroups := []ParallelGroup{{"task1"}, {"task2"}}
	assert.Equal(t, expectedGroups, newPlan.ParallelGroups, "Parallel groups should be maintained")
}

func TestCallingPlanMinusTaskZero_SameKeyDifferentValues(t *testing.T) {
	// Create a test plan engine
	p := &PlanEngine{
		Logger: zerolog.Nop(), // Silent logger for tests
	}

	// Create a plan with TaskZero and two tasks with the same key but different values
	planJSON := `{
		"tasks": [
			{
				"id": "task0",
				"input": {
					"productId": "laptop-1",
					"userId": "user-1"
				}
			},
			{
				"id": "task1",
				"service": "s_byohsjbzdqmldroxueutktsnelgf",
				"input": {
					"action": "checkAvailability",
					"productId": "$task0.productId"
				}
			},
			{
				"id": "task2",
				"service": "s_byohsjbzdqmldroxueutktsnelgf",
				"input": {
					"action": "reserveProduct",
					"productId": "$task0.productId"
				}
			}
		],
		"parallel_groups": [
			["task1"],
			["task2"]
		]
	}`

	var plan ExecutionPlan
	err := json.Unmarshal([]byte(planJSON), &plan)
	require.NoError(t, err, "Failed to unmarshal test plan")

	// Execute the function
	taskZero, newPlan := p.callingPlanMinusTaskZero(&plan)

	// Find task1 and task2 in the new plan
	var task1, task2 *SubTask
	for _, task := range newPlan.Tasks {
		if task.ID == "task1" {
			task1 = task
		} else if task.ID == "task2" {
			task2 = task
		}
	}

	require.NotNil(t, task1, "task1 not found in the new plan")
	require.NotNil(t, task2, "task2 not found in the new plan")

	// Verify TaskZero has both action values with different keys
	// The first one should be just "action"
	assert.Equal(t, "checkAvailability", taskZero.Input["action"],
		"TaskZero should have action field with value 'checkAvailability'")

	// The second one should have a unique name like "action_2"
	var secondActionKey string
	var foundSecondAction bool

	for key, val := range taskZero.Input {
		if key != "action" && key != "productId" && key != "userId" {
			if strVal, ok := val.(string); ok && strVal == "reserveProduct" {
				secondActionKey = key
				foundSecondAction = true
				break
			}
		}
	}

	assert.True(t, foundSecondAction, "Should find a second action key in TaskZero")
	assert.Contains(t, secondActionKey, "action", "Second action key should contain 'action'")
	assert.Equal(t, "reserveProduct", taskZero.Input[secondActionKey],
		"Second action key should have value 'reserveProduct'")

	// Verify each task references the correct field in TaskZero
	assert.Equal(t, "$task0.action", task1.Input["action"],
		"task1 should reference TaskZero.action")
	assert.Equal(t, fmt.Sprintf("$task0.%s", secondActionKey), task2.Input["action"],
		"task2 should reference the unique action field in TaskZero")
}
