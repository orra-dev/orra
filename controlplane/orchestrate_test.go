/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeJSONOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		err      string
	}{
		{
			name:     "basic json extraction",
			input:    "```json{\n  \"key\": \"value\"\n}```",
			expected: "{\n  \"key\": \"value\"\n}",
			err:      "",
		},
		{
			name:     "json with surrounding text",
			input:    "Here's some text before\n```json {\n  \"type\": \"object\",\n  \"name\": \"test\"\n} ```\nAnd text after",
			expected: "{\n  \"type\": \"object\",\n  \"name\": \"test\"\n}",
			err:      "",
		},
		{
			name:     "no json markers",
			input:    "Just some regular text",
			expected: "",
			err:      "cannot find opening JSON marker",
		},
		{
			name:     "missing json prefix",
			input:    "``````",
			expected: "``````",
			err:      "cannot find opening JSON marker",
		},
		{
			name:     "has json prefix but no json content",
			input:    "```json```",
			expected: "",
			err:      "cannot parse invalid JSON",
		},
		{
			name:     "missing closing marker",
			input:    "```json\n{\"key\": \"value\"}",
			expected: "",
			err:      "cannot find closing JSON marker",
		},
		{
			name:     "multiple json blocks",
			input:    "```json\n{\"first\": true}```\nSome text\n```json{\"second\": true}```",
			expected: "{\"first\": true}",
		},
		{
			name:     "json with extra whitespace",
			input:    "\n\n  ```json\n{\n    \"key\": \"value\"\n  }\n```  \n\n",
			expected: "{\n    \"key\": \"value\"\n  }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractValidJSONOutput(tt.input)
			if tt.err != "" {
				assert.ErrorContains(t, err, tt.err)
			} else {
				assert.Equal(t, tt.expected, result, "The extracted JSON should match the expected output")
			}
		})
	}
}
