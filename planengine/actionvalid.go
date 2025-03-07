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
// - Arrays of primitives (including nested arrays of primitives)
// - No objects or arrays containing objects
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

	case map[string]interface{}, map[string]string:
		// Objects (maps) are not valid
		return fmt.Errorf("field '%s' contains complex object which is not allowed", field)

	default:
		// Check for array types
		val := reflect.ValueOf(value)

		// Check if it's any kind of array/slice
		if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
			// For empty arrays, we can't determine element type, so assume it's valid
			if val.Len() == 0 {
				return nil
			}

			// Check the first element to see if it's an object/map
			firstElem := val.Index(0).Interface()
			elemKind := reflect.ValueOf(firstElem).Kind()

			if elemKind == reflect.Map || elemKind == reflect.Struct {
				return fmt.Errorf("field '%s' contains array of objects which is not allowed", field)
			}

			// If the element is another slice, we need to check it recursively
			if elemKind == reflect.Slice || elemKind == reflect.Array {
				// Recursively check each element in the array
				for i := 0; i < val.Len(); i++ {
					elemVal := val.Index(i).Interface()
					if err := ValidateActionParamValue(fmt.Sprintf("%s[%d]", field, i), elemVal); err != nil {
						return err
					}
				}
			}

			// If we got here, it's a valid array of primitives or primitive arrays
			return nil
		}

		// If we can't recognize the type as valid, reject it
		return fmt.Errorf("field '%s' has unsupported type %T", field, value)
	}
}
