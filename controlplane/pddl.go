/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/rs/zerolog"
)

// PDDLDomainGenerator handles converting grounding specs to PDDL
type PDDLDomainGenerator struct {
	planAction    string
	executionPlan *ExecutionPlan
	groundingSpec *GroundingSpec
	matcher       *Matcher
	logger        zerolog.Logger
}

func NewPDDLDomainGenerator(action string, plan *ExecutionPlan, spec *GroundingSpec, matcher *Matcher, logger zerolog.Logger) *PDDLDomainGenerator {
	return &PDDLDomainGenerator{
		planAction:    action,
		executionPlan: plan,
		groundingSpec: spec,
		matcher:       matcher,
		logger:        logger,
	}
}

// ShouldGenerateDomain checks if we should proceed with PDDL generation
func (g *PDDLDomainGenerator) ShouldGenerateDomain() bool {
	// Skip if no grounding spec was used
	if g.groundingSpec == nil {
		return false
	}

	// Skip if execution plan doesn't reference grounding
	if g.executionPlan.GroundingID == "" {
		return false
	}

	return true
}

// GenerateDomain creates PDDL domain file content from matching use-case
func (g *PDDLDomainGenerator) GenerateDomain(ctx context.Context) (string, error) {
	if !g.ShouldGenerateDomain() {
		return "", fmt.Errorf("no domain grounding available for PDDL generation")
	}

	// Find matching use-case based on execution plan
	useCase, err := g.findMatchingUseCaseAgainstAction(ctx)
	if err != nil {
		return "", fmt.Errorf("no matching use-case found for execution plan")
	}

	// Build PDDL domain content
	var domain strings.Builder

	// Add domain header
	domain.WriteString(fmt.Sprintf("(define (domain %s)\n", g.groundingSpec.Domain))
	domain.WriteString("  (:requirements :strips :typing)\n\n")

	// Add types section based on params
	g.addTypes(&domain, useCase)

	// Add predicates section based on capabilities
	g.addPredicates(&domain, useCase)

	// Add actions section based on use-case
	g.addActions(&domain, useCase)

	domain.WriteString(")\n")

	return domain.String(), nil
}

func (g *PDDLDomainGenerator) findMatchingUseCaseAgainstAction(ctx context.Context) (*GroundingUseCase, error) {
	for _, useCase := range g.groundingSpec.UseCases {
		match, err := g.planActionMatches(ctx, useCase.Action)
		if err != nil {
			return nil, err
		}
		if match {
			return &useCase, nil
		}
	}

	return nil, fmt.Errorf("no matching use-case found for execution plan")
}

func (g *PDDLDomainGenerator) planActionMatches(ctx context.Context, useCaseAction string) (bool, error) {
	normalizedPlanAction := g.normalizeActionPattern(g.planAction)
	normalizedUseCase := g.normalizeActionPattern(useCaseAction)

	matches, score, err := g.matcher.MatchTexts(ctx, normalizedPlanAction, normalizedUseCase, 0.85)
	if err != nil {
		return false, fmt.Errorf("failed to match actions: %w", err)
	}

	g.logger.Debug().
		Str("planAction", g.planAction).
		Str("useCaseAction", normalizedUseCase).
		Float64("score", score).
		Bool("matches", matches).
		Msg("Action similarity check")

	return matches, nil
}

func (g *PDDLDomainGenerator) normalizeActionPattern(pattern string) string {
	// Remove variable placeholders
	vars := regexp.MustCompile(`\{[^}]+\}`)
	return vars.ReplaceAllString(pattern, "")
}

func (g *PDDLDomainGenerator) addTypes(domain *strings.Builder, useCase *GroundingUseCase) {
	typeHierarchy := make(map[string][]string)
	seenTypes := make(map[string]bool)

	// Helper to add a type and its parent to the hierarchy
	addToHierarchy := func(typeName string) {
		if !seenTypes[typeName] {
			seenTypes[typeName] = true
			parentType := g.inferParentType(typeName)
			if parentType != "" {
				typeHierarchy[typeName] = []string{parentType}
				// Ensure parent type is also added as a base type
				if !seenTypes[parentType] {
					seenTypes[parentType] = true
					typeHierarchy[parentType] = nil
				}
			} else {
				typeHierarchy[typeName] = nil
			}
		}
	}

	// Process use case params
	for _, paramValue := range useCase.Params {
		paramType := g.inferTypeFromParam(paramValue, g.executionPlan.Tasks)
		if paramType != "" {
			addToHierarchy(paramType)
		}
	}

	// Process service schemas
	for _, task := range g.executionPlan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) {
			continue
		}

		// Process input properties
		if task.ExpectedInput.Properties != nil {
			for propName, propSpec := range task.ExpectedInput.Properties {
				pddlType := g.inferDetailedType(propSpec.Type, propName)
				addToHierarchy(pddlType)
			}
		}

		// Process output properties
		if task.ExpectedOutput.Properties != nil {
			for propName, propSpec := range task.ExpectedOutput.Properties {
				pddlType := g.inferDetailedType(propSpec.Type, propName)
				addToHierarchy(pddlType)
			}
		}
	}

	// Only write types section if we have types to write
	if len(typeHierarchy) > 0 {
		domain.WriteString("  (:types\n")

		// Write base types first
		baseTypes := make([]string, 0)
		for typeName, parents := range typeHierarchy {
			if len(parents) == 0 && typeName != "object" { // Exclude 'object' from base types
				baseTypes = append(baseTypes, typeName)
			}
		}
		sort.Strings(baseTypes)
		if len(baseTypes) > 0 {
			domain.WriteString(fmt.Sprintf("    %s - object\n", strings.Join(baseTypes, " ")))
		}

		// Write derived types
		derivedTypes := make([]string, 0)
		for typeName, parents := range typeHierarchy {
			if len(parents) > 0 {
				for _, parent := range parents {
					derivedTypes = append(derivedTypes, fmt.Sprintf("    %s - %s", typeName, parent))
				}
			}
		}
		sort.Strings(derivedTypes)
		for _, line := range derivedTypes {
			domain.WriteString(line + "\n")
		}

		domain.WriteString("  )\n\n")
	}
}

// inferDetailedType provides more specific type inference based on property name and context
func (g *PDDLDomainGenerator) inferDetailedType(jsonType, propName string) string {
	propNameLower := strings.ToLower(propName)

	switch jsonType {
	case "string":
		// Infer semantic types based on property name patterns
		switch {
		case strings.HasSuffix(propNameLower, "id"):
			prefix := strings.TrimSuffix(propNameLower, "id")
			if prefix != "" {
				return prefix + "-id"
			}
			return "id"
		case strings.HasSuffix(propNameLower, "location"):
			prefix := strings.TrimSuffix(propNameLower, "location")
			if prefix != "" {
				return prefix + "-location"
			}
			return "location"
		case strings.HasSuffix(propNameLower, "status"):
			prefix := strings.TrimSuffix(propNameLower, "status")
			if prefix != "" {
				return prefix + "-status"
			}
			return "status"
		default:
			return "object"
		}
	case "number", "integer":
		if strings.Contains(propNameLower, "amount") ||
			strings.Contains(propNameLower, "price") ||
			strings.Contains(propNameLower, "cost") {
			return "monetary-value"
		}
		return "number"
	case "boolean":
		return "boolean"
	case "array":
		return "collection"
	default:
		return "object"
	}
}

// inferParentType determines the parent type based on the type name
func (g *PDDLDomainGenerator) inferParentType(typeName string) string {
	switch {
	case strings.HasSuffix(typeName, "-id"):
		return "id"
	case strings.HasSuffix(typeName, "-location"):
		return "location"
	case strings.HasSuffix(typeName, "-status"):
		return "status"
	case strings.HasSuffix(typeName, "-value"):
		return "number"
	default:
		return "" // No parent type
	}
}

func (g *PDDLDomainGenerator) inferTypeFromParam(param interface{}, subtasks []*SubTask) string {
	// First check service schemas for type information
	for _, task := range subtasks {
		if task.ExpectedInput.Properties != nil {
			for propName, propSpec := range task.ExpectedInput.Properties {
				// Check if this parameter is used in the input
				if inputVal, ok := task.Input[propName]; ok {
					if fmt.Sprintf("%v", inputVal) == fmt.Sprintf("%v", param) {
						// Use our new inferDetailedType method
						return g.inferDetailedType(propSpec.Type, propName)
					}
				}
			}
		}
	}

	// Fallback to type inference based on the parameter value
	switch v := param.(type) {
	case float64, int, int64:
		return g.inferDetailedType("number", "")
	case bool:
		return g.inferDetailedType("boolean", "")
	case string:
		// Try to infer type from the string value itself
		strValue := v
		switch {
		case strings.Contains(strings.ToLower(strValue), "id"):
			return g.inferDetailedType("string", strValue)
		default:
			return g.inferDetailedType("string", "")
		}
	default:
		return g.inferDetailedType("", "")
	}
}

func (g *PDDLDomainGenerator) addPredicates(_ *strings.Builder, _ *GroundingUseCase) {}

func (g *PDDLDomainGenerator) addActions(_ *strings.Builder, _ *GroundingUseCase) {}
