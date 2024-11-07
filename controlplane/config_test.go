/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		expected string
	}{
		{"Registered", Registered, "registered"},
		{"Pending", Pending, "pending"},
		{"Processing", Processing, "processing"},
		{"Completed", Completed, "completed"},
		{"Failed", Failed, "failed"},
		{"NotActionable", NotActionable, "not_actionable"},
		{"Paused", Paused, "paused"},
		{"Invalid Status", Status(999), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestStatus_MarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		status      Status
		expected    string
		shouldError bool
	}{
		{
			name:     "Registered status",
			status:   Registered,
			expected: `"registered"`,
		},
		{
			name:     "Pending status",
			status:   Pending,
			expected: `"pending"`,
		},
		{
			name:     "Processing status",
			status:   Processing,
			expected: `"processing"`,
		},
		{
			name:     "Completed status",
			status:   Completed,
			expected: `"completed"`,
		},
		{
			name:     "Failed status",
			status:   Failed,
			expected: `"failed"`,
		},
		{
			name:     "NotActionable status",
			status:   NotActionable,
			expected: `"not_actionable"`,
		},
		{
			name:     "Paused status",
			status:   Paused,
			expected: `"paused"`,
		},
		{
			name:        "Invalid status",
			status:      Status(999),
			expected:    `""`,
			shouldError: false, // Note: This implementation marshals invalid status as empty string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.status)
			if tt.shouldError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(data))
		})
	}
}

func TestStatus_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    Status
		expectError bool
	}{
		{
			name:     "registered status",
			input:    `"registered"`,
			expected: Registered,
		},
		{
			name:     "pending status",
			input:    `"pending"`,
			expected: Pending,
		},
		{
			name:     "processing status",
			input:    `"processing"`,
			expected: Processing,
		},
		{
			name:     "completed status",
			input:    `"completed"`,
			expected: Completed,
		},
		{
			name:     "failed status",
			input:    `"failed"`,
			expected: Failed,
		},
		{
			name:     "not_actionable status",
			input:    `"not_actionable"`,
			expected: NotActionable,
		},
		{
			name:     "paused status",
			input:    `"paused"`,
			expected: Paused,
		},
		{
			name:     "uppercase status",
			input:    `"COMPLETED"`,
			expected: Completed,
		},
		{
			name:     "status with whitespace",
			input:    `" pending "`,
			expected: Pending,
		},
		{
			name:        "invalid status string",
			input:       `"invalid_status"`,
			expectError: true,
		},
		{
			name:        "empty string",
			input:       `""`,
			expectError: true,
		},
		{
			name:        "invalid json",
			input:       `{invalid}`,
			expectError: true,
		},
		{
			name:        "numeric value",
			input:       `1`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status Status
			err := json.Unmarshal([]byte(tt.input), &status)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, status)
		})
	}
}

func TestStatus_InStruct(t *testing.T) {
	type TestStruct struct {
		ID        string        `json:"id"`
		Status    Status        `json:"status"`
		Statuses  []Status      `json:"statuses,omitempty"`
		Timestamp time.Time     `json:"timestamp"`
		Duration  time.Duration `json:"duration"`
	}

	now := time.Now().UTC()
	duration := 5 * time.Second

	t.Run("marshal and unmarshal struct with status", func(t *testing.T) {
		original := TestStruct{
			ID:        "test1",
			Status:    Completed,
			Statuses:  []Status{Pending, Processing, Completed},
			Timestamp: now,
			Duration:  duration,
		}

		// Marshal
		data, err := json.Marshal(original)
		require.NoError(t, err)

		// Verify JSON structure
		expected := fmt.Sprintf(
			`{"id":"test1","status":"completed","statuses":["pending","processing","completed"],"timestamp":"%s","duration":%d}`,
			now.Format(time.RFC3339Nano),
			duration,
		)
		assert.JSONEq(t, expected, string(data))

		// Unmarshal
		var decoded TestStruct
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		// Compare fields
		assert.Equal(t, original.ID, decoded.ID)
		assert.Equal(t, original.Status, decoded.Status)
		assert.Equal(t, original.Statuses, decoded.Statuses)
		assert.Equal(t, original.Timestamp.Unix(), decoded.Timestamp.Unix())
		assert.Equal(t, original.Duration, decoded.Duration)
	})
}

func TestServiceType_Marshaling(t *testing.T) {
	t.Run("marshal service type", func(t *testing.T) {
		tests := []struct {
			stype    ServiceType
			expected string
		}{
			{Agent, `"agent"`},
			{Service, `"service"`},
			{ServiceType(999), `""`},
		}

		for _, tt := range tests {
			data, err := json.Marshal(tt.stype)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(data))
		}
	})

	t.Run("unmarshal service type", func(t *testing.T) {
		tests := []struct {
			input       string
			expected    ServiceType
			shouldError bool
		}{
			{`"agent"`, Agent, false},
			{`"service"`, Service, false},
			{`"AGENT"`, Agent, false},
			{`"SERVICE"`, Service, false},
			{`" agent "`, Agent, false},
			{`"invalid"`, ServiceType(0), true},
			{`""`, ServiceType(0), true},
			{`123`, ServiceType(0), true},
		}

		for _, tt := range tests {
			var st ServiceType
			err := json.Unmarshal([]byte(tt.input), &st)
			if tt.shouldError {
				assert.Error(t, err)
				continue
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, st)
		}
	})
}
