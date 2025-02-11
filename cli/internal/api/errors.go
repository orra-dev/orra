/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package api

import (
	"fmt"
	"strings"
)

// Error represents a structured error from the API
type Error struct {
	Kind    string `json:"kind"`
	Param   string `json:"param"`
	Message string `json:"message"`
}

// ErrorResponse wraps the error response from the API
type ErrorResponse struct {
	Error Error `json:"error"`
}

// FriendlyError represents an error with a user-friendly message
type FriendlyError struct {
	Original error
	UserMsg  string
}

func (e FriendlyError) Error() string {
	return e.UserMsg
}

// FormatAPIError attempts to format the Orra API error response into a user-friendly message
func FormatAPIError(errRes ErrorResponse, resource string) error {
	// Build user-friendly message based on error type
	switch errRes.Error.Kind {
	case "invalid operation":
		switch errRes.Error.Param {
		case "version":
			if strings.Contains(errRes.Error.Message, "already has") {
				return &FriendlyError{
					UserMsg: extractExistingResourceMessage(errRes.Error.Message, resource),
				}
			}
		}
	case "validation":
		return &FriendlyError{
			UserMsg: fmt.Sprintf("Invalid %s: %s", resource, errRes.Error.Message),
		}
	case "not found":
		return &FriendlyError{
			UserMsg: fmt.Sprintf("%s not found", resource),
		}
	case "unauthorized":
		return &FriendlyError{
			UserMsg: "Not authorized. Please check your API key",
		}
	}

	// Fallback to original message if no specific handling
	return &FriendlyError{
		UserMsg: errRes.Error.Message,
	}
}

// extractExistingResourceMessage creates a user-friendly message for "already exists" errors
func extractExistingResourceMessage(msg string, resource string) string {
	// Extract name and version if present in error message
	nameStart := strings.Index(msg, "with name ")
	versionStart := strings.Index(msg, "and version ")

	if nameStart != -1 && versionStart != -1 {
		name := msg[nameStart+10 : versionStart-1] // +10 to skip "with name ", -1 to remove space
		version := msg[versionStart+11:]           // +11 to skip "and version "
		return fmt.Sprintf("A %s named '%s' with version%s already exists",
			resource, name, version)
	}

	return fmt.Sprintf("This %s already exists", resource)
}
