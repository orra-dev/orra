/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"fmt"
	"testing"
)

// Test cases to verify the implementation
func TestExtractDependencyID(t *testing.T) {
	testCases := []struct {
		name           string
		input          any
		expectedDepID  string
		expectedDepKey string
	}{
		{"has dependency id", "$task0.param1", "task0", "param1"},
		{"has complex dependency id", "$complex-task-id.field", "complex-task-id", "field"},
		{"has no dependency", "notadependency", "", ""},
		{"has invalid dependency", "$.invalid", "", ""},
		{"has dependency but no param", "$task0", "", ""},
		{"empty input", "", "", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualDepID, actualDepKey := extractDependencyIDAndKey(tc.input)
			if actualDepID != tc.expectedDepID {
				panic(fmt.Sprintf("Failed: input=%q, got=(%q, %q), want=(%q, %q)",
					tc.input,
					actualDepID,
					actualDepKey,
					tc.expectedDepID,
					tc.expectedDepKey))
			}
		})
	}
}
