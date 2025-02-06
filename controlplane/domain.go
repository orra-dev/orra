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
)

// extractVariables returns all variables in {braces} from an action string
func extractVariables(action string) []string {
	re := regexp.MustCompile(`\{([^}]+)}`)
	matches := re.FindAllStringSubmatch(action, -1)

	vars := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			vars = append(vars, match[1])
		}
	}
	return vars
}

// validateActionParams checks if params match action variables
func validateActionParams(action string, params map[string]string) error {
	variables := extractVariables(action)
	if len(variables) == 0 {
		return nil // No variables in action, params not required
	}

	if params == nil || len(params) == 0 {
		return fmt.Errorf("params required when action contains variables: %v", variables)
	}

	var missingParams []string
	for _, v := range variables {
		if _, ok := params[v]; !ok {
			missingParams = append(missingParams, v)
		}
	}

	if len(missingParams) > 0 {
		return fmt.Errorf("missing required param(s): %v", missingParams)
	}

	return nil
}

// validateCapabilities checks if capabilities are valid
func validateCapabilities(caps []string) error {
	if caps == nil || len(caps) == 0 {
		return fmt.Errorf("at least one capability is required")
	}

	for i, capabilities := range caps {
		if strings.TrimSpace(capabilities) == "" {
			return fmt.Errorf("capability at index %d is empty", i)
		}
	}

	return nil
}

// Validate checks if the ActionExample is valid
func (e *ActionExample) Validate() error {
	// Validate action
	if strings.TrimSpace(e.Action) == "" {
		return fmt.Errorf("action: cannot be blank")
	}

	// Validate params against action variables
	if err := validateActionParams(e.Action, e.Params); err != nil {
		return fmt.Errorf("params: %v", err)
	}

	// Validate capabilities
	if err := validateCapabilities(e.Capabilities); err != nil {
		return fmt.Errorf("capabilities: %v", err)
	}

	// Validate intent
	if strings.TrimSpace(e.Intent) == "" {
		return fmt.Errorf("intent: cannot be blank")
	}

	return nil
}

// validateConstraints checks if constraints are valid when provided
func validateConstraints(constraints []string) error {
	if constraints == nil {
		return nil
	}

	if len(constraints) == 0 {
		return fmt.Errorf("if provided, must contain at least one constraint")
	}

	for i, constraint := range constraints {
		if strings.TrimSpace(constraint) == "" {
			return fmt.Errorf("constraint at index %d cannot be empty", i)
		}
	}
	return nil
}

// Validate validates the DomainExample
func (d *DomainExample) Validate() error {
	// Validate name
	if len(d.Name) < 3 || len(d.Name) > 63 {
		return fmt.Errorf("name: must be between 3 and 63 characters")
	}
	nameRegex := regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)
	if !nameRegex.MatchString(d.Name) {
		return fmt.Errorf("name: must consist of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character")
	}

	// Validate domain
	if strings.TrimSpace(d.Domain) == "" {
		return fmt.Errorf("domain: cannot be blank")
	}

	// Validate version
	if strings.TrimSpace(d.Version) == "" {
		return fmt.Errorf("version: cannot be blank")
	}

	// Validate examples
	if len(d.Examples) == 0 {
		return fmt.Errorf("examples: cannot be empty")
	}
	for i, example := range d.Examples {
		if err := example.Validate(); err != nil {
			return fmt.Errorf("examples[%d]: %v", i, err)
		}
	}

	// Validate constraints if provided
	if len(d.Constraints) > 0 {
		if err := validateConstraints(d.Constraints); err != nil {
			return fmt.Errorf("constraints: %v", err)
		}
	}

	return nil
}
