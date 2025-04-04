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
			name: "all action params present in task0 (array format)",
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
				{"field": "query", "value": "I need a used laptop"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount: 0,
			wantError:  false,
		},
		{
			name: "missing params - first validation (retry 0) - array format",
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
			errorContains: "action parameters missing from TaskZero: [budget category]",
		},
		{
			name: "missing params - second validation (retry 1) - array format",
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
			errorContains: "Action parameters should only be added to TaskZero and never have their values embedded into another parameter",
		},
		{
			name: "missing params - third validation (retry 2) - array format",
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
			retryCount:    2,
			wantError:     true,
			errorContains: "A downstream task may require the missing action params as part of their service's input schema - these are the targeted services: [s_aodlipsfcgzmnirkyhyrlmdavwag]",
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
			errorContains: "task zero not found in execution plan",
		},
		{
			name: "some action params missing from task0 - multiple services - array format",
			executionPlan: `{
				"tasks": [
					{
						"id": "task0",
						"input": {
							"query": "I need a used laptop with budget 800",
							"category": "laptop"
						}
					},
					{
						"id": "task1",
						"service": "s_service1",
						"input": {
							"query": "$task0.query"
						}
					},
					{
						"id": "task2",
						"service": "s_service2",
						"input": {
							"category": "$task0.category"
						}
					}
				],
				"parallel_groups": [
					["task1"],
					["task2"]
				]
			}`,
			actionParams: `[
				{"field": "query", "value": "I need a used laptop"},
				{"field": "budget", "value": 800},
				{"field": "category", "value": "laptop"}
			]`,
			retryCount:    2,
			wantError:     true,
			errorContains: "these are the targeted services: [s_service1 s_service2]",
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
			} else {
				assert.NoError(t, err, "Expected no error but got one")
			}
		})
	}
}
