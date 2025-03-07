/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionParams_ValueTypes(t *testing.T) {
	t.Run("Valid value types", func(t *testing.T) {
		// Test valid basic JSON types
		validCases := []struct {
			name   string
			params ActionParams
		}{
			{
				name: "String values",
				params: ActionParams{
					{Field: "name", Value: "John Doe"},
					{Field: "title", Value: "Software Engineer"},
				},
			},
			{
				name: "Number values",
				params: ActionParams{
					{Field: "age", Value: 30},
					{Field: "salary", Value: 75000.50},
				},
			},
			{
				name: "Boolean values",
				params: ActionParams{
					{Field: "active", Value: true},
					{Field: "verified", Value: false},
				},
			},
			{
				name: "Null value",
				params: ActionParams{
					{Field: "middleName", Value: nil},
				},
			},
			{
				name: "Array of primitive values",
				params: ActionParams{
					{Field: "tags", Value: []string{"go", "cloud", "api"}},
					{Field: "scores", Value: []int{85, 90, 78}},
				},
			},
			{
				name: "Mixed primitive types",
				params: ActionParams{
					{Field: "name", Value: "Jane"},
					{Field: "age", Value: 25},
					{Field: "isPremium", Value: true},
					{Field: "tags", Value: []string{"member", "active"}},
				},
			},
			{
				name: "Nested arrays of primitives",
				params: ActionParams{
					{Field: "matrix", Value: [][]int{{1, 2}, {3, 4}}},
					{Field: "options", Value: [][]string{{"yes", "no"}, {"true", "false"}}},
				},
			},
		}

		for _, tc := range validCases {
			t.Run(tc.name, func(t *testing.T) {
				// Test marshaling to JSON
				jsonBytes, err := json.Marshal(tc.params)
				require.NoError(t, err, "Failed to marshal valid params")

				// Test unmarshaling from JSON
				var unmarshaled ActionParams
				err = json.Unmarshal(jsonBytes, &unmarshaled)
				require.NoError(t, err, "Failed to unmarshal valid params")

				// Compare values
				for i, param := range tc.params {
					// Using string comparison for values since custom comparison would be complex
					expectedJSON, err := json.Marshal(param.Value)
					require.NoError(t, err)

					actualJSON, err := json.Marshal(unmarshaled[i].Value)
					require.NoError(t, err)

					assert.Equal(t, param.Field, unmarshaled[i].Field, "Field name should match")
					assert.JSONEq(t, string(expectedJSON), string(actualJSON), "Value content should match")
				}
			})
		}
	})

	t.Run("Invalid value types", func(t *testing.T) {
		// Create test cases with invalid values
		invalidCases := []struct {
			name          string
			jsonStr       string
			expectedError string
		}{
			{
				name:          "Object value",
				jsonStr:       `[{"field":"user","value":{"name":"John","age":30}}]`,
				expectedError: "contains complex object",
			},
			{
				name:          "Array of objects",
				jsonStr:       `[{"field":"users","value":[{"name":"John"},{"name":"Jane"}]}]`,
				expectedError: "contains array of objects",
			},
			{
				name:          "Nested object in array",
				jsonStr:       `[{"field":"data","value":["string",{"key":"value"},123]}]`,
				expectedError: "contains complex object",
			},
		}

		// Test invalid cases
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				var params ActionParams
				err := json.Unmarshal([]byte(tc.jsonStr), &params)
				assert.NoError(t, err, "JSON unmarshaling should work for any valid JSON")

				validErr := ValidateActionParamValue(params[0].Field, params[0].Value)
				assert.Error(t, validErr, tc.expectedError)
			})
		}
	})
}

// TestActionParamsValidator tests the validation function separately
func TestActionParamsValidator(t *testing.T) {
	testCases := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{"String value", "test", true},
		{"Integer value", 123, true},
		{"Float value", 123.45, true},
		{"Boolean value", true, true},
		{"Nil value", nil, true},
		{"String slice", []string{"a", "b"}, true},
		{"Int slice", []int{1, 2, 3}, true},
		{"Interface slice with primitives", []interface{}{"a", 1, true}, true},
		{"2D string slice", [][]string{{"a", "b"}, {"c", "d"}}, true},
		{"2D int slice", [][]int{{1, 2}, {3, 4}}, true},
		{"Object", map[string]interface{}{"key": "value"}, false},
		{"Empty object", map[string]interface{}{}, false},
		{"Array with object", []interface{}{"a", map[string]interface{}{"key": "value"}}, false},
		{"Array of objects", []map[string]interface{}{{"key": "value"}}, false},
		{"Array with nested object", []interface{}{[]interface{}{"a", map[string]interface{}{"key": "value"}}}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateActionParamValue("", tc.value)
			assert.Equal(t, tc.expected, err == nil)
		})
	}
}

// TestActionParams_ValidateAll tests the validation of an entire ActionParams slice
func TestActionParams_ValidateAll(t *testing.T) {
	testCases := []struct {
		name     string
		params   ActionParams
		expected bool
	}{
		{
			name: "All valid values",
			params: ActionParams{
				{Field: "name", Value: "John"},
				{Field: "age", Value: 30},
				{Field: "tags", Value: []string{"go", "developer"}},
			},
			expected: true,
		},
		{
			name: "Mixed valid and invalid values",
			params: ActionParams{
				{Field: "name", Value: "John"},
				{Field: "profile", Value: map[string]interface{}{"title": "Developer"}},
			},
			expected: false,
		},
		{
			name: "All invalid values",
			params: ActionParams{
				{Field: "user", Value: map[string]interface{}{"name": "John"}},
				{Field: "items", Value: []map[string]interface{}{{"id": 1}}},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateActionParams(tc.params)
			assert.Equal(t, tc.expected, err == nil)
		})
	}
}

// TestActionParams_Json tests the Json method ensuring it properly marshals the params
func TestActionParams_Json(t *testing.T) {
	params := ActionParams{
		{Field: "name", Value: "John"},
		{Field: "age", Value: 30},
		{Field: "active", Value: true},
		{Field: "tags", Value: []string{"go", "developer"}},
	}

	rawJSON, err := params.Json()
	require.NoError(t, err, "Json() should marshal without error")

	// Unmarshal back to verify
	var unmarshaled ActionParams
	err = json.Unmarshal(rawJSON, &unmarshaled)
	require.NoError(t, err, "Should unmarshal correctly")

	// Check field names
	assert.Equal(t, len(params), len(unmarshaled), "Should have same number of params")
	for i, param := range params {
		assert.Equal(t, param.Field, unmarshaled[i].Field, "Field name should match")
	}

	// Check values by marshaling them individually
	for i, param := range params {
		expectedJSON, err := json.Marshal(param.Value)
		require.NoError(t, err)

		actualJSON, err := json.Marshal(unmarshaled[i].Value)
		require.NoError(t, err)

		assert.JSONEq(t, string(expectedJSON), string(actualJSON), "Value content should match")
	}
}

// TestActionParams_String tests the String method
func TestActionParams_String(t *testing.T) {
	params := ActionParams{
		{Field: "name", Value: "John"},
		{Field: "age", Value: 30},
	}

	result := params.String()
	expected := "name::age"
	assert.Equal(t, expected, result, "String() should join field names with '::'")
}
