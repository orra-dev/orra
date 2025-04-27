/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDurationUnmarshal(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{`"5s"`, 5 * time.Second},
		{`5000000000`, 5 * time.Second},
		{`{"Duration":5000000000}`, 5 * time.Second}, // Legacy format
	}

	for _, tt := range tests {
		var d Duration
		err := json.Unmarshal([]byte(tt.input), &d)
		if err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if time.Duration(d) != tt.expected {
			t.Fatalf("expected %v, got %v", tt.expected, d)
		}
	}
}
