/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
