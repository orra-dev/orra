/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"regexp"

	v "github.com/RussellLuo/validating/v3"
)

func validateSpec(spec *Spec) v.Schema {
	baseSchema := v.Schema{
		v.F("type", spec.Type): v.All(
			v.Nonzero[string]().Msg("type is required"),
			v.In("object", "string", "number", "integer", "boolean", "array").Msg("invalid type"),
		),
	}

	switch spec.Type {
	case "object":
		baseSchema[v.F("properties", spec.Properties)] = v.All(
			v.Is(func(m map[string]Spec) bool {
				return len(m) > 0
			}).Msg("properties are required for object type and must have at least one entry"),
			v.Map(func(m map[string]Spec) map[string]v.Validator {
				schemas := make(map[string]v.Validator)
				for key, propSpec := range m {
					propSpecCopy := propSpec
					schemas[key] = validateSpec(&propSpecCopy)
				}
				return schemas
			}),
		)
	case "array":
		if spec.Items == nil {
			baseSchema[v.F("items", spec.Items)] = v.Is(func(i *Spec) bool {
				return false
			}).Msg("items is required for array type")
		} else {
			baseSchema[v.F("items", spec.Items)] = validateSpec(spec.Items)
		}
	}

	return baseSchema
}

func (s Spec) Validation() v.Schema {
	return validateSpec(&s)
}

func (s ServiceSchema) Validation() v.Schema {
	return v.Schema{
		v.F("input", s.Input): v.All(
			validateSpec(&s.Input),
			v.Is(func(spec Spec) bool {
				return spec.Type == "object"
			}).Msg("top-level input spec must be of type 'object'"),
		),
		v.F("output", s.Output): v.All(
			validateSpec(&s.Output),
			v.Is(func(spec Spec) bool {
				return spec.Type == "object"
			}).Msg("top-level output spec must be of type 'object'"),
		),
	}
}

func (si *ServiceInfo) Validation() v.Schema {
	return v.Schema{
		v.F("name", si.Name): v.All(
			v.LenString(3, 63).Msg("name must be between 3 and 63 characters"),
			v.Match(regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)).
				Msg("name must consist of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"),
		),
		v.F("description", si.Description): v.Nonzero[string]().Msg("empty description"),
		v.F("schema", si.Schema):           si.Schema.Validation(),
	}
}
