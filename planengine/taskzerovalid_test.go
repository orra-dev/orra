/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTaskZeroParams(t *testing.T) {
	// Create a dummy PlanEngine to call the method.
	cp := &PlanEngine{
		Logger: zerolog.Nop(), // Silent logger for tests
	}

	testCases := []struct {
		name          string
		executionPlan string
		actionParams  string
		retryCount    int
		wantError     bool
		errorContains string
		notContains   string // Error should NOT contain this string
	}{
		{
			name: "all action params present in task0 (map format)",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "I need a used laptop",
							"budget": 800,
							"category": "laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_aodlipsfcgzmnirkyhyrlmdavwag",
						"input": {
							"query": "$task0.query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "I need a used laptop for college that is powerful enough for programming"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount: 0,
			wantError:  false,
		},
		{
			name: "params embedded in query - first validation (retry 0)",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "I need a used laptop for college that is powerful enough for programming budget: 800 category: laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_aodlipsfcgzmnirkyhyrlmdavwag",
						"input": {
							"query": "$task0.query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "I need a used laptop for college that is powerful enough for programming"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount:    0,
			wantError:     true,
			errorContains: "parameters budget, category are missing from task0 inputs (embedded in query",
		},
		{
			name: "params embedded with different patterns - retry 0",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "I need a used laptop with budget=800 and category is laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_aodlipsfcgzmnirkyhyrlmdavwag",
						"input": {
							"query": "$task0.query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "I need a used laptop"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount:    0,
			wantError:     true,
			errorContains: "parameters budget, category are missing from task0 inputs (embedded in query",
		},
		{
			name: "params truly missing (not embedded) - retry 0",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "I need a used laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_aodlipsfcgzmnirkyhyrlmdavwag",
						"input": {
							"query": "$task0.query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "I need a used laptop"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount:    0,
			wantError:     true,
			errorContains: "parameters budget, category are missing from task0 inputs",
			notContains:   "embedded in",
		},
		{
			name: "params embedded in query - second validation (retry 1)",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "I need a used laptop for college that is powerful enough for programming budget: 800 category: laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_aodlipsfcgzmnirkyhyrlmdavwag",
						"input": {
							"query": "$task0.query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "I need a used laptop for college that is powerful enough for programming"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount:    1,
			wantError:     true,
			errorContains: "They appear to be embedded in query",
		},
		{
			name: "params truly missing (not embedded) - retry 1",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "I need a used laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_aodlipsfcgzmnirkyhyrlmdavwag",
						"input": {
							"query": "$task0.query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "I need a used laptop"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount:    1,
			wantError:     true,
			errorContains: "parameters budget, category are missing from task0",
			notContains:   "appear to be embedded",
		},
		{
			name: "params embedded in query - final validation (retry 2)",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "I need a used laptop for college that is powerful enough for programming budget: 800 category: laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_aodlipsfcgzmnirkyhyrlmdavwag",
						"serviceName": "Product Search",
						"input": {
							"query": "$task0.query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "I need a used laptop for college that is powerful enough for programming"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount:    2,
			wantError:     true,
			errorContains: "PROBLEM: The generated execution plan is embedding parameters within other fields",
		},
		{
			name: "params truly missing (not embedded) - retry 2",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "I need a used laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_aodlipsfcgzmnirkyhyrlmdavwag",
						"serviceName": "Product Search",
						"input": {
							"query": "$task0.query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "I need a used laptop"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount:    2,
			wantError:     true,
			errorContains: "PROBLEM: The generated execution plan is missing required parameters",
			notContains:   "embedding parameters",
		},
		{
			name: "no task0 in execution plan",
			executionPlan: `{
				"tasks": [
					{
						"id": "task1",
						"service": "s_aodlipsfcgzmnirkyhyrlmdavwag",
						"input": {
							"query": "Some query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams:  `[{"field": "query", "value": "Original query"}]`,
			retryCount:    0,
			wantError:     true,
			errorContains: "task0 not found in execution plan",
		},
		{
			name: "multiple parameters embedded in query",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "Find laptops with budget: 800 price_max: 1000 price_min: 500 and category is laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_service1",
						"serviceName": "Product Search",
						"input": {
							"query": "$task0.query"
						}
					}
				],
				"parallel_groups": [
					["task1"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "Find laptops"},
				{"field": "budget", "value": 800},
				{"field": "price_max", "value": 1000},
				{"field": "price_min", "value": 500},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount:    2,
			wantError:     true,
			errorContains: "query contains [budget, price_max, price_min, category]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse JSON data
			var plan ExecutionPlan
			err := json.Unmarshal([]byte(tc.executionPlan), &plan)
			require.NoError(t, err, "Failed to unmarshal execution plan")

			actionParams := json.RawMessage(tc.actionParams)

			// Call the validation function with retry count
			_, err = cp.validateTaskZeroParams(&plan, actionParams, tc.retryCount)

			if tc.wantError {
				assert.Error(t, err, "Expected an error but got none")
				if tc.errorContains != "" {
					assert.True(t, strings.Contains(err.Error(), tc.errorContains),
						"Expected error to contain %q, but got: %q", tc.errorContains, err.Error())
				}
				if tc.notContains != "" {
					assert.False(t, strings.Contains(err.Error(), tc.notContains),
						"Expected error NOT to contain %q, but got: %q", tc.notContains, err.Error())
				}
			} else {
				assert.NoError(t, err, "Expected no error but got one")
			}
		})
	}
}

func TestValidateNoCompositeTask0Refs(t *testing.T) {
	// Create a minimal PlanEngine for testing
	p := &PlanEngine{}

	tests := []struct {
		name        string
		plan        *ExecutionPlan
		retryCount  int
		wantStatus  Status
		wantErr     bool
		errContains string
	}{
		{
			name: "valid plan with single task0 references",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"query":    "$task0.query",
							"budget":   "$task0.budget",
							"category": "$task0.category",
						},
					},
				},
			},
			retryCount: 0,
			wantStatus: Continue,
			wantErr:    false,
		},
		{
			name: "invalid plan with composite task0 references - first attempt",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"query": "$task0.query budget: $task0.budget category: $task0.category",
						},
					},
				},
			},
			retryCount:  0,
			wantStatus:  Failed,
			wantErr:     true,
			errContains: "found composite task0 references in tasks",
		},
		{
			name: "invalid plan with composite task0 references - retry 1",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"query": "$task0.query budget: $task0.budget category: $task0.category",
						},
					},
				},
			},
			retryCount:  1,
			wantStatus:  Failed,
			wantErr:     true,
			errContains: "found composite task0 references that should be separate parameters",
		},
		{
			name: "invalid plan with composite task0 references - retry 2+",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"query": "$task0.query budget: $task0.budget category: $task0.category",
						},
					},
				},
			},
			retryCount:  2,
			wantStatus:  Failed,
			wantErr:     true,
			errContains: "ORCHESTRATION ERROR: Found composite task0 references in downstream tasks",
		},
		{
			name: "multiple composite references in different tasks",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":     "I need a laptop",
							"budget":    "800",
							"category":  "laptop",
							"condition": "new",
						},
					},
					{
						ID:      "task1",
						Service: "service1",
						Input: map[string]interface{}{
							"query": "$task0.query budget: $task0.budget",
						},
					},
					{
						ID:      "task2",
						Service: "service2",
						Input: map[string]interface{}{
							"description": "Looking for $task0.category with condition: $task0.condition",
						},
					},
				},
			},
			retryCount:  2,
			wantStatus:  Failed,
			wantErr:     true,
			errContains: "ORCHESTRATION ERROR: Found composite task0 references in downstream tasks",
		},
		{
			name: "non-string values should not be checked",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":  "I need a laptop",
							"budget": 800,
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"query":  "$task0.query",
							"budget": 800,  // numeric, not checked
							"flag":   true, // boolean, not checked
						},
					},
				},
			},
			retryCount: 0,
			wantStatus: Continue,
			wantErr:    false,
		},
		{
			name: "empty plan should be valid",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{},
			},
			retryCount: 0,
			wantStatus: Continue,
			wantErr:    false,
		},
		{
			name: "plan with only task0 should be valid",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query": "I need a laptop",
						},
					},
				},
			},
			retryCount: 0,
			wantStatus: Continue,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := p.validateNoCompositeTaskZeroRefs(tt.plan, tt.retryCount)

			// Check status
			assert.Equal(t, tt.wantStatus, status, "Status should match expected")

			// Check error presence
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNoCompositeTaskZeroRefs_SingleReferenceWithText(t *testing.T) {
	// Create a minimal PlanEngine for testing
	p := &PlanEngine{}

	tests := []struct {
		name        string
		plan        *ExecutionPlan
		retryCount  int
		wantStatus  Status
		wantErr     bool
		errContains string
	}{
		{
			name: "valid plan with exact task0 references",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"query":    "$task0.query",
							"budget":   "$task0.budget",
							"category": "$task0.category",
						},
					},
				},
			},
			retryCount: 0,
			wantStatus: Continue,
			wantErr:    false,
		},
		{
			name: "single reference with additional text - first attempt",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"query": "$task0.query budget",
						},
					},
				},
			},
			retryCount:  0,
			wantStatus:  Failed,
			wantErr:     true,
			errContains: "found invalid task0 references in tasks",
		},
		{
			name: "single reference with additional text - retry 1",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"query": "$task0.query budget",
						},
					},
				},
			},
			retryCount:  1,
			wantStatus:  Failed,
			wantErr:     true,
			errContains: "contains a reference with additional text",
		},
		{
			name: "single reference with additional text - retry 2+",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"query": "$task0.query budget",
						},
					},
				},
			},
			retryCount:  2,
			wantStatus:  Failed,
			wantErr:     true,
			errContains: "ORCHESTRATION ERROR: Found invalid task0 references in downstream tasks",
		},
		{
			name: "mixed cases - multiple references and single refs with text",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "service1",
						Input: map[string]interface{}{
							"query": "$task0.query budget: $task0.budget", // Multiple refs
						},
					},
					{
						ID:      "task2",
						Service: "service2",
						Input: map[string]interface{}{
							"description": "$task0.category with extras", // Single ref with text
						},
					},
				},
			},
			retryCount:  2,
			wantStatus:  Failed,
			wantErr:     true,
			errContains: "ORCHESTRATION ERROR: Found invalid task0 references in downstream tasks",
		},
		{
			name: "various formats of invalid references",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							"q1": "$task0.query budget",                   // Text after
							"q2": "Searching for $task0.query",            // Text before
							"q3": "Find $task0.query products",            // Text before and after
							"q4": " $task0.query ",                        // With spaces
							"q5": "Use $task0.budget and $task0.category", // Multiple refs with text
						},
					},
				},
			},
			retryCount:  1,
			wantStatus:  Failed,
			wantErr:     true,
			errContains: "found invalid task0 references that need correction",
		},
		{
			name: "parameter references without additional text are valid",
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID: TaskZero,
						Input: map[string]interface{}{
							"query":    "I need a laptop",
							"budget":   "800",
							"category": "laptop",
						},
					},
					{
						ID:      "task1",
						Service: "some_service",
						Input: map[string]interface{}{
							// All valid references
							"q1": "$task0.query",
							"q2": "$task0.budget",
							"q3": "$task0.category",
						},
					},
				},
			},
			retryCount: 0,
			wantStatus: Continue,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := p.validateNoCompositeTaskZeroRefs(tt.plan, tt.retryCount)

			// Check status
			assert.Equal(t, tt.wantStatus, status, "Status should match expected")

			// Check error presence
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Also test the helper function
func TestIsExactTaskZeroRef(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"$task0.query", true},
		{"$task0.complex_field_name", true},
		{"$task0.123", true},
		{"$task0.field_123", true},

		// Invalid cases
		{"$task0.query extra", false},
		{"prefix $task0.query", false},
		{"$task0.query suffix", false},
		{"mixed $task0.query text", false},
		{" $task0.query", false},
		{"$task0.query ", false},
		{"$task0.query\n", false},
		{"$task0.query,$task0.budget", false},
		{"not a reference", false},
		{"$task.query", false}, // Missing 0
		{"task0.query", false}, // Missing $
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isExactTaskZeroRef(tt.input)
			assert.Equal(t, tt.expected, result,
				"isExactTaskZeroRef(%q) should return %v", tt.input, tt.expected)
		})
	}
}
