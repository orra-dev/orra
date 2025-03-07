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
	"strings"
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
			got, err := extractTaskZeroInput(tt.content)

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

func TestExtractParamMappings_ArrayValues(t *testing.T) {
	tests := []struct {
		name         string
		actionParams ActionParams
		task0Input   map[string]interface{}
		want         []TaskZeroCacheMapping
		wantErr      bool
	}{
		{
			name: "array action parameter",
			actionParams: ActionParams{
				{Field: "tags", Value: []interface{}{"swatch", "collectible", "red"}},
				{Field: "customerId", Value: "cust12345"},
			},
			task0Input: map[string]interface{}{
				"customerID": "cust12345",
				"tagList":    []interface{}{"swatch", "collectible", "red"},
			},
			want: []TaskZeroCacheMapping{
				{Field: "customerID", ActionField: "customerId", Value: "cust12345"},
				{Field: "tagList", ActionField: "tags", Value: `["swatch","collectible","red"]`},
			},
		},
		{
			name: "nested object action parameter",
			actionParams: ActionParams{
				{Field: "filter", Value: map[string]interface{}{"color": "red", "size": "medium"}},
				{Field: "customerId", Value: "cust12345"},
			},
			task0Input: map[string]interface{}{
				"customerID":     "cust12345",
				"searchCriteria": map[string]interface{}{"color": "red", "size": "medium"},
			},
			want: []TaskZeroCacheMapping{
				{Field: "customerID", ActionField: "customerId", Value: "cust12345"},
				{Field: "searchCriteria", ActionField: "filter", Value: `{"color":"red","size":"medium"}`},
			},
		},
		{
			name: "mixed array types",
			actionParams: ActionParams{
				{Field: "mixed", Value: []interface{}{"string", 123, true, nil}},
			},
			task0Input: map[string]interface{}{
				"mixedList": []interface{}{"string", 123, true, nil},
			},
			want: []TaskZeroCacheMapping{
				{Field: "mixedList", ActionField: "mixed", Value: `["string",123,true,null]`},
			},
		},
		{
			name: "array with nested objects",
			actionParams: ActionParams{
				{Field: "products", Value: []interface{}{
					map[string]interface{}{"id": "p1", "name": "Product 1"},
					map[string]interface{}{"id": "p2", "name": "Product 2"},
				}},
			},
			task0Input: map[string]interface{}{
				"productList": []interface{}{
					map[string]interface{}{"id": "p1", "name": "Product 1"},
					map[string]interface{}{"id": "p2", "name": "Product 2"},
				},
			},
			want: []TaskZeroCacheMapping{
				{Field: "productList", ActionField: "products", Value: `[{"id":"p1","name":"Product 1"},{"id":"p2","name":"Product 2"}]`},
			},
		},
		{
			name: "empty array",
			actionParams: ActionParams{
				{Field: "emptyList", Value: []interface{}{}},
			},
			task0Input: map[string]interface{}{
				"emptyItems": []interface{}{},
			},
			want: []TaskZeroCacheMapping{
				{Field: "emptyItems", ActionField: "emptyList", Value: `[]`},
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

			// Compare each field individually to help debugging
			if len(got) != len(tt.want) {
				t.Errorf("expected %d mappings, got %d", len(tt.want), len(got))
			} else {
				for i := range got {
					assert.Equal(t, tt.want[i].Field, got[i].Field, "Field mismatch")
					assert.Equal(t, tt.want[i].ActionField, got[i].ActionField, "ActionField mismatch")

					// For complex values like arrays, compare JSON representations
					if strings.HasPrefix(tt.want[i].Value, "[") || strings.HasPrefix(tt.want[i].Value, "{") {
						var wantVal, gotVal interface{}
						err = json.Unmarshal([]byte(tt.want[i].Value), &wantVal)
						require.NoError(t, err)

						err = json.Unmarshal([]byte(got[i].Value), &gotVal)
						require.NoError(t, err)

						assert.Equal(t, wantVal, gotVal, "Value JSON mismatch")
					} else {
						assert.Equal(t, tt.want[i].Value, got[i].Value, "Value mismatch")
					}
				}
			}
		})
	}
}

func TestSubstituteTask0Params_ArrayValues(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		originalInput string
		newParams     string
		mappings      []TaskZeroCacheMapping
		want          map[string]interface{}
	}{
		{
			name: "substitute array parameter",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "tagList": ["swatch", "collectible"]
                        }
                    },
                    {
                        "id": "task1",
                        "service": "TagAnalyzer",
                        "input": {
                            "tags": "$task0.tagList"
                        }
                    }
                ],
                "parallel_groups": [["task1"]]
            }`,
			originalInput: `{"tagList":["swatch","collectible"]}`,
			newParams:     `[{"field":"tags","value":["swatch","collectible","red"]}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "tagList", ActionField: "tags", Value: `["swatch","collectible"]`},
			},
			want: map[string]interface{}{
				"tagList": []interface{}{"swatch", "collectible", "red"},
			},
		},
		{
			name: "substitute object parameter",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "searchCriteria": {"color": "blue", "size": "small"}
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			originalInput: `{"searchCriteria":{"color":"blue","size":"small"}}`,
			newParams:     `[{"field":"filter","value":{"color":"red","size":"medium"}}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "searchCriteria", ActionField: "filter", Value: `{"color":"blue","size":"small"}`},
			},
			want: map[string]interface{}{
				"searchCriteria": map[string]interface{}{"color": "red", "size": "medium"},
			},
		},
		{
			name: "substitute array with nested objects",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "productList": [{"id":"p1","name":"Product 1"}]
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			originalInput: `{"productList":[{"id":"p1","name":"Product 1"}]}`,
			newParams:     `[{"field":"products","value":[{"id":"p1","name":"Product 1"},{"id":"p2","name":"Product 2"}]}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "productList", ActionField: "products", Value: `[{"id":"p1","name":"Product 1"}]`},
			},
			want: map[string]interface{}{
				"productList": []interface{}{
					map[string]interface{}{"id": "p1", "name": "Product 1"},
					map[string]interface{}{"id": "p2", "name": "Product 2"},
				},
			},
		},
		{
			name: "substitute mixed types with primitives and arrays",
			content: `{
                "tasks": [
                    {
                        "id": "task0",
                        "input": {
                            "customerId": "cust12345",
                            "tagList": ["watch"]
                        }
                    }
                ],
                "parallel_groups": []
            }`,
			originalInput: `{"customerId":"cust12345","tagList":["watch"]}`,
			newParams:     `[{"field":"customerId","value":"cust98765"},{"field":"tags","value":["vintage","collectible"]}]`,
			mappings: []TaskZeroCacheMapping{
				{Field: "customerId", ActionField: "customerId", Value: "cust12345"},
				{Field: "tagList", ActionField: "tags", Value: `["watch"]`},
			},
			want: map[string]interface{}{
				"customerId": "cust98765",
				"tagList":    []interface{}{"vintage", "collectible"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := substituteTask0Params(tt.content, []byte(tt.originalInput), []byte(tt.newParams), tt.mappings)
			require.NoError(t, err)

			var gotJSON map[string]interface{}
			err = json.Unmarshal([]byte(got), &gotJSON)
			require.NoError(t, err)

			// Extract task0 input from the result
			tasks := gotJSON["tasks"].([]interface{})
			task0 := tasks[0].(map[string]interface{})
			task0Input := task0["input"].(map[string]interface{})

			// Verify against expected values
			for key, expectedValue := range tt.want {
				assert.Contains(t, task0Input, key, "Key should exist in task0 input")

				// For complex types like arrays and objects, compare as JSON
				switch expected := expectedValue.(type) {
				case []interface{}, map[string]interface{}:
					expectedJSON, err := json.Marshal(expected)
					require.NoError(t, err)

					actualJSON, err := json.Marshal(task0Input[key])
					require.NoError(t, err)

					var expectedObj, actualObj interface{}
					err = json.Unmarshal(expectedJSON, &expectedObj)
					require.NoError(t, err)

					err = json.Unmarshal(actualJSON, &actualObj)
					require.NoError(t, err)

					assert.Equal(t, expectedObj, actualObj,
						"Value mismatch for key %s. Expected: %s, Got: %s",
						key, string(expectedJSON), string(actualJSON))
				default:
					assert.Equal(t, expectedValue, task0Input[key],
						"Value mismatch for key %s", key)
				}
			}
		})
	}
}
