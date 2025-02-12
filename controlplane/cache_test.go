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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTask0Input(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid simple task0",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "param1": "value1"
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			want: `{"param1":"value1"}`,
		},
		{
			name: "valid complex task0",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "param1": "value1",
                            "param2": 42,
                            "param3": ["a","b","c"],
                            "param4": {"nested":true}
                        }
                    },
                    {
                        "id": "task1",
                        "service": "ServiceA",
                        "input": {
                            "data": "$task0.param1"
                        }
                    }
                ],
                "parallel_groups": [["task1"]]
            }`,
			want: `{"param1":"value1","param2":42,"param3":["a","b","c"],"param4":{"nested":true}}`,
		},
		{
			name: "task0 with dependent inputs",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "param1": "value1",
                            "param2": "$other.value"
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			want: `{"param1":"value1","param2":"$other.value"}`,
		},
		{
			name: "no task0",
			content: `{
                "tasks": [
                    {
                        "id": "task1",
                        "service": "ServiceA",
                        "input": {
                            "data": "value1"
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			wantErr:     true,
			errContains: "task0 not found",
		},
		{
			name:        "invalid json",
			content:     `{invalid json}`,
			wantErr:     true,
			errContains: "failed to parse",
		},
		{
			name: "empty task0 input",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {}
                    }
                ],
                "parallel_groups": []
            }`,
			want: `{}`,
		},
		{
			name: "task0 with null values",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "param1": null
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			want: `{"param1":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractTask0Input(tt.content)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)

			// Compare JSON structures
			var gotJSON, wantJSON interface{}
			err = json.Unmarshal(got, &gotJSON)
			require.NoError(t, err)

			err = json.Unmarshal([]byte(tt.want), &wantJSON)
			require.NoError(t, err)

			assert.Equal(t, wantJSON, gotJSON)
		})
	}
}

func TestSubstituteTask0Params(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		originalInput string
		newParams     string
		mappings      []TaskZeroCacheMapping
		want          map[string]string
	}{
		{
			name: "substitute customer id into message field",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "message": "cust12345"
                        }
                    },
                    {
                        "id": "task1",
                        "service": "Echo",
                        "input": {
                            "data": "$task0.message"
                        }
                    }
                ],
                "parallel_groups": [["task1"]]
            }`,
			originalInput: `{"message":"cust12345"}`,
			newParams:     `[{"field":"customerId","value":"cust98765"},{"field":"productDescription","value":"Red shoes"}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "message", ActionField: "customerId"},
			},
			want: map[string]string{
				"message": "cust98765",
			},
		},
		{
			name: "substitute multiple fields",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "text": "Peanuts collectible",
                            "id": "cust12345"
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			originalInput: `{"text":"Peanuts collectible","id":"cust12345"}`,
			newParams:     `[{"field":"customerId","value":"cust98765"},{"field":"productDescription","value":"Vintage comics"}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "text", ActionField: "productDescription"},
				{Field: "id", ActionField: "customerId"},
			},
			want: map[string]string{
				"id":   "cust98765",
				"text": "Vintage comics",
			},
		},
		{
			name: "preserve non-mapped fields",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "constant": "some-fixed-value",
                            "message": "cust12345"
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			originalInput: `{"constant":"some-fixed-value","message":"cust12345"}`,
			newParams:     `[{"field":"customerId","value":"cust98765"}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "message", ActionField: "customerId"},
			},
			want: map[string]string{
				"constant": "some-fixed-value",
				"message":  "cust98765",
			},
		},
	}

	for _, tt := range tests {
		got, err := substituteTask0Params(tt.content, []byte(tt.originalInput), []byte(tt.newParams), tt.mappings)
		require.NoError(t, err)

		var gotJSON map[string]any
		err = json.Unmarshal([]byte(got), &gotJSON)
		require.NoError(t, err)

		for k, v := range tt.want {
			t.Run(fmt.Sprintf("%s_has %+v", tt.name, v), func(t *testing.T) {
				assert.Equal(t, v, gotJSON["tasks"].([]any)[0].(map[string]any)["input"].(map[string]any)[k])
			})
		}
	}
}

func TestSubstituteTask0ParamsErrors(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		originalInput string
		newParams     string
		mappings      []TaskZeroCacheMapping
		errContains   string
	}{
		{
			name: "missing mapped parameter",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "message": "cust12345"
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			originalInput: `{"message":"cust12345"}`,
			newParams:     `[{"field":"otherField","value":"someValue"}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "message", ActionField: "customerId"},
			},
			errContains: "missing required action parameter: customerId",
		},
		{
			name: "invalid action params JSON",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "message": "cust12345"
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			originalInput: `{"message":"cust12345"}`,
			newParams:     `[{"field":"customerId", bad json}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "message", ActionField: "customerId"},
			},
			errContains: "failed to parse new action params",
		},
		{
			name: "no task0",
			content: `{
                "tasks": [
                    {
                        "id": "task1",
                        "input": {
                            "message": "value1"
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			originalInput: `{"message":"cust12345"}`,
			newParams:     `[{"field":"customerId","value":"cust98765"}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "message", ActionField: "customerId"},
			},
			errContains: "task0 not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := substituteTask0Params(tt.content, []byte(tt.originalInput), []byte(tt.newParams), tt.mappings)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestExtractParamMappings(t *testing.T) {
	tests := []struct {
		name         string
		actionParams ActionParams
		task0Input   map[string]interface{}
		want         []TaskZeroCacheMapping
		wantErr      bool
	}{
		{
			name: "simple field mapping",
			actionParams: ActionParams{
				{Field: "customerId", Value: "cust12345"},
				{Field: "productDescription", Value: "Red shoes"},
			},
			task0Input: map[string]interface{}{
				"message": "cust12345",
			},
			want: []TaskZeroCacheMapping{
				{Field: "message", ActionField: "customerId", Value: "cust12345"},
			},
		},
		{
			name: "multiple field mappings",
			actionParams: ActionParams{
				{Field: "customerId", Value: "cust12345"},
				{Field: "productDescription", Value: "Red shoes"},
			},
			task0Input: map[string]interface{}{
				"text": "Red shoes",
				"id":   "cust12345",
			},
			want: []TaskZeroCacheMapping{
				{Field: "id", ActionField: "customerId", Value: "cust12345"},
				{Field: "text", ActionField: "productDescription", Value: "Red shoes"},
			},
		},
		{
			name: "no mappings found",
			actionParams: ActionParams{
				{Field: "customerId", Value: "cust12345"},
			},
			task0Input: map[string]interface{}{
				"message": "something else",
			},
			want: nil,
		},
		{
			name: "handle non-string task0 values",
			actionParams: ActionParams{
				{Field: "customerId", Value: "cust12345"},
			},
			task0Input: map[string]interface{}{
				"message": "cust12345",
				"count":   42,
			},
			want: []TaskZeroCacheMapping{
				{Field: "message", ActionField: "customerId", Value: "cust12345"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractParamMappings(tt.actionParams, tt.task0Input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Sort both slices for consistent comparison
			sort.Slice(got, func(i, j int) bool {
				return got[i].Field < got[j].Field
			})
			sort.Slice(tt.want, func(i, j int) bool {
				return tt.want[i].Field < tt.want[j].Field
			})

			assert.Equal(t, tt.want, got)
		})
	}
}
