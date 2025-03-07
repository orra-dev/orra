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
		{"Object", map[string]interface{}{"key": "value"}, true},
		{"Empty list", []interface{}{}, true},
		{"Empty object", map[string]interface{}{}, true},
		{"Array with object", []interface{}{"a", map[string]interface{}{"key": "value"}}, true},
		{"Array of objects", []map[string]interface{}{{"key": "value"}}, true},
		{"Array with nested object", []interface{}{[]interface{}{"a", map[string]interface{}{"key": "value"}}}, true},
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
			expected: true,
		},
		{
			name: "All invalid values",
			params: ActionParams{
				{Field: "user", Value: map[string]interface{}{"name": "John"}},
				{Field: "items", Value: []map[string]interface{}{{"id": 1}}},
			},
			expected: true,
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

func TestActionParamsValidator_WithObjects(t *testing.T) {
	testCases := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		// Basic primitive types (should all pass)
		{"String value", "test", true},
		{"Integer value", 123, true},
		{"Float value", 123.45, true},
		{"Boolean value", true, true},
		{"Nil value", nil, true},

		// Array types (should all pass)
		{"String slice", []string{"a", "b"}, true},
		{"Int slice", []int{1, 2, 3}, true},
		{"Interface slice with primitives", []interface{}{"a", 1, true}, true},
		{"2D string slice", [][]string{{"a", "b"}, {"c", "d"}}, true},
		{"2D int slice", [][]int{{1, 2}, {3, 4}}, true},

		// Object types (should now pass)
		{"Simple object", map[string]interface{}{"key": "value"}, true},
		{"Empty object", map[string]interface{}{}, true},
		{"Object with number", map[string]interface{}{"count": 42}, true},
		{"Object with boolean", map[string]interface{}{"active": true}, true},
		{"Object with null", map[string]interface{}{"optional": nil}, true},
		{"Object with array", map[string]interface{}{"tags": []string{"a", "b"}}, true},

		// Nested object types (should now pass)
		{"Nested object", map[string]interface{}{
			"user": map[string]interface{}{
				"name": "John",
				"age":  30,
			},
		}, true},
		{"Object with array of objects", map[string]interface{}{
			"users": []map[string]interface{}{
				{"name": "John", "age": 30},
				{"name": "Jane", "age": 25},
			},
		}, true},

		// Complex nested structures (should now pass)
		{"Array with objects", []interface{}{
			"string value",
			map[string]interface{}{"key": "value"},
			42,
		}, true},
		{"Array of objects", []map[string]interface{}{
			{"name": "John", "age": 30},
			{"name": "Jane", "age": 25},
		}, true},
		{"Deeply nested structure", map[string]interface{}{
			"company": map[string]interface{}{
				"name": "Acme Inc",
				"employees": []map[string]interface{}{
					{
						"name":   "John",
						"skills": []string{"Go", "Python"},
						"projects": []map[string]interface{}{
							{"name": "API", "status": "completed"},
							{"name": "UI", "status": "in-progress"},
						},
					},
					{
						"name":   "Jane",
						"skills": []string{"Java", "C++"},
						"projects": []map[string]interface{}{
							{"name": "Database", "status": "completed"},
						},
					},
				},
			},
		}, true},

		// Still invalid types
		{"Function", func() {}, false},
		{"Channel", make(chan int), false},
		{"Complex number", complex(1, 2), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateActionParamValue("test", tc.value)
			assert.Equal(t, tc.expected, err == nil, "Validation result should match expected")
			if tc.expected {
				assert.NoError(t, err, "Should validate without error")
			} else {
				assert.Error(t, err, "Should return validation error")
			}
		})
	}
}

func TestActionParams_ValidateAll_WithObjects(t *testing.T) {
	testCases := []struct {
		name     string
		params   ActionParams
		expected bool
	}{
		{
			name: "All valid primitive values",
			params: ActionParams{
				{Field: "name", Value: "John"},
				{Field: "age", Value: 30},
				{Field: "tags", Value: []string{"go", "developer"}},
			},
			expected: true,
		},
		{
			name: "With simple object (should now pass)",
			params: ActionParams{
				{Field: "name", Value: "John"},
				{Field: "profile", Value: map[string]interface{}{"title": "Developer"}},
			},
			expected: true,
		},
		{
			name: "With object and array (should now pass)",
			params: ActionParams{
				{Field: "user", Value: map[string]interface{}{"name": "John"}},
				{Field: "scores", Value: []int{85, 90, 78}},
			},
			expected: true,
		},
		{
			name: "With array of objects (should now pass)",
			params: ActionParams{
				{Field: "name", Value: "John"},
				{Field: "friends", Value: []map[string]interface{}{
					{"name": "Alice", "age": 28},
					{"name": "Bob", "age": 32},
				}},
			},
			expected: true,
		},
		{
			name: "Complex nested structure",
			params: ActionParams{
				{Field: "customer", Value: map[string]interface{}{
					"name": "John Doe",
					"address": map[string]interface{}{
						"street": "123 Main St",
						"city":   "Anytown",
						"zip":    "12345",
					},
					"orders": []map[string]interface{}{
						{
							"id":     "ORD-001",
							"items":  []string{"Product A", "Product B"},
							"total":  125.99,
							"status": "shipped",
						},
						{
							"id":     "ORD-002",
							"items":  []string{"Product C"},
							"total":  59.99,
							"status": "pending",
						},
					},
				}},
			},
			expected: true,
		},
		{
			name: "Invalid non-JSON type",
			params: ActionParams{
				{Field: "name", Value: "John"},
				{Field: "callback", Value: func() {}},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateActionParams(tc.params)
			assert.Equal(t, tc.expected, err == nil, "Validation result should match expected")
			if !tc.expected {
				assert.Error(t, err, "Should return validation error")
			}
		})
	}
}

func TestJsonMarshalUnmarshal_WithObjects(t *testing.T) {
	testCases := []struct {
		name   string
		params ActionParams
	}{
		{
			name: "Simple object",
			params: ActionParams{
				{Field: "user", Value: map[string]interface{}{
					"name": "John Doe",
					"age":  30,
				}},
			},
		},
		{
			name: "Array of objects",
			params: ActionParams{
				{Field: "products", Value: []map[string]interface{}{
					{"id": "p1", "name": "Product 1", "price": 29.99},
					{"id": "p2", "name": "Product 2", "price": 39.99},
				}},
			},
		},
		{
			name: "Nested complex structure",
			params: ActionParams{
				{Field: "order", Value: map[string]interface{}{
					"id": "ord123",
					"customer": map[string]interface{}{
						"id":   "cust456",
						"name": "Jane Smith",
					},
					"items": []map[string]interface{}{
						{"product": "p1", "quantity": 2},
						{"product": "p2", "quantity": 1},
					},
					"shipping": map[string]interface{}{
						"method":  "express",
						"address": "123 Main St",
						"cost":    15.99,
					},
				}},
			},
		},
		{
			name: "Mixed types",
			params: ActionParams{
				{Field: "name", Value: "John Doe"},
				{Field: "age", Value: 30},
				{Field: "active", Value: true},
				{Field: "tags", Value: []string{"customer", "premium"}},
				{Field: "address", Value: map[string]interface{}{
					"street": "123 Main St",
					"city":   "Anytown",
					"zip":    "12345",
				}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test marshaling to JSON
			jsonBytes, err := json.Marshal(tc.params)
			require.NoError(t, err, "Failed to marshal params with objects")

			// Test unmarshaling from JSON
			var unmarshaled ActionParams
			err = json.Unmarshal(jsonBytes, &unmarshaled)
			require.NoError(t, err, "Failed to unmarshal params with objects")

			// Compare values
			assert.Equal(t, len(tc.params), len(unmarshaled), "Should have same number of params")

			for i, param := range tc.params {
				// Check field name matches
				assert.Equal(t, param.Field, unmarshaled[i].Field, "Field name should match")

				// For comparing values, marshal both to JSON and compare
				expectedJSON, err := json.Marshal(param.Value)
				require.NoError(t, err)

				actualJSON, err := json.Marshal(unmarshaled[i].Value)
				require.NoError(t, err)

				assert.JSONEq(t, string(expectedJSON), string(actualJSON), "Value content should match")
			}

			// Validate the params with our updated validator
			err = ValidateActionParams(unmarshaled)
			assert.NoError(t, err, "Unmarshaled params should validate successfully")
		})
	}
}
