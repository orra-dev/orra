/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"fmt"
	"reflect"
)

// ValidateActionParams validates that all parameters in ActionParams contain only
// acceptable JSON types and returns an error if invalid types are found
func ValidateActionParams(params ActionParams) error {
	for _, param := range params {
		if err := ValidateActionParamValue(param.Field, param.Value); err != nil {
			return err
		}
	}
	return nil
}

// ValidateActionParamValue validates that a value contains only acceptable JSON types:
// - Primitives (string, number, boolean, null)
// - Arrays of any valid JSON values (including nested arrays)
// - Objects (maps) with any valid JSON values
func ValidateActionParamValue(field string, value interface{}) error {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string, float64, int, int64, float32, bool, uint, uint64, uint32:
		// Basic primitives are valid
		return nil

	case []interface{}:
		// Check each element in the array
		for i, element := range v {
			if err := ValidateActionParamValue(fmt.Sprintf("%s[%d]", field, i), element); err != nil {
				return fmt.Errorf("invalid array element at %s: %w", field, err)
			}
		}
		return nil

	case map[string]interface{}:
		// Objects (maps) are now valid
		// Check each value in the object
		for key, element := range v {
			if err := ValidateActionParamValue(fmt.Sprintf("%s.%s", field, key), element); err != nil {
				return fmt.Errorf("invalid object property at %s.%s: %w", field, key, err)
			}
		}
		return nil

	default:
		// Check for array types and object types using reflection
		val := reflect.ValueOf(value)
		kind := val.Kind()

		// Handle arrays/slices
		if kind == reflect.Slice || kind == reflect.Array {
			// For empty arrays, we can't determine element type, so assume it's valid
			if val.Len() == 0 {
				return nil
			}

			// Recursively check each element in the array
			for i := 0; i < val.Len(); i++ {
				elemVal := val.Index(i).Interface()
				if err := ValidateActionParamValue(fmt.Sprintf("%s[%d]", field, i), elemVal); err != nil {
					return err
				}
			}
			return nil
		}

		// Handle maps/objects
		if kind == reflect.Map {
			// For empty maps, assume it's valid
			if val.Len() == 0 {
				return nil
			}

			// Iterate through map keys and validate each value
			for _, key := range val.MapKeys() {
				elemVal := val.MapIndex(key).Interface()
				keyStr := fmt.Sprintf("%v", key.Interface())
				if err := ValidateActionParamValue(fmt.Sprintf("%s.%s", field, keyStr), elemVal); err != nil {
					return err
				}
			}
			return nil
		}

		// If we can't recognize the type as valid, reject it
		return fmt.Errorf("field '%s' has unsupported type %T", field, value)
	}
}
