/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

// TestGenerateServiceKeyFormat repeatedly calls GenerateServiceKey (100 times)
// and verifies that each key starts with "s_" and contains only lowercase letters.
func TestGenerateServiceKeyFormat(t *testing.T) {
	cp := &ControlPlane{}
	pattern := `^s_[a-z]+$`
	regex, err := regexp.Compile(pattern)
	assert.NoError(t, err, "failed to compile regex pattern")

	for i := 0; i < 100; i++ {
		key := cp.GenerateServiceKey()
		assert.True(t, strings.HasPrefix(key, "s_"), "key %q should start with 's_'", key)
		assert.True(t, regex.MatchString(key), "key %q does not match required pattern %q", key, pattern)
	}
}

// TestGenerateServiceKeyUniqueness ensures that multiple generated keys are unique.
func TestGenerateServiceKeyUniqueness(t *testing.T) {
	cp := &ControlPlane{}
	keyCount := 1000
	keys := make(map[string]struct{}, keyCount)

	for i := 0; i < keyCount; i++ {
		key := cp.GenerateServiceKey()
		if _, exists := keys[key]; exists {
			t.Fatalf("duplicate key found: %s", key)
		}
		keys[key] = struct{}{}
	}
}
