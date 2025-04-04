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
