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
	"strings"

	"github.com/rs/zerolog"
)

type PddlDomainGenerator struct {
	planAction    string
	executionPlan *ExecutionPlan
	groundingSpec *GroundingSpec
	matcher       SimilarityMatcher
	logger        zerolog.Logger
}

func NewPddlDomainGenerator(action string, plan *ExecutionPlan, spec *GroundingSpec, matcher SimilarityMatcher, logger zerolog.Logger) *PddlDomainGenerator {
	return &PddlDomainGenerator{
		planAction:    action,
		executionPlan: plan,
		groundingSpec: spec,
		matcher:       matcher,
		logger:        logger,
	}
}

func (g *PddlDomainGenerator) ShouldGenerateDomain() bool {
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

func (g *PddlDomainGenerator) findMatchingUseCaseAgainstAction(ctx context.Context) (*GroundingUseCase, error) {
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

func (g *PddlDomainGenerator) planActionMatches(ctx context.Context, useCaseAction string) (bool, error) {
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

func (g *PddlDomainGenerator) normalizeActionPattern(pattern string) string {
	// Remove variable placeholders
	vars := regexp.MustCompile(`\{[^}]+\}`)
	return vars.ReplaceAllString(pattern, "")
}

func (g *PddlDomainGenerator) GenerateDomain(ctx context.Context) (string, error) {
	if !g.ShouldGenerateDomain() {
		return "", fmt.Errorf("no domain grounding available for PDDL generation")
	}

	// Find matching use-case based on execution plan
	useCase, err := g.findMatchingUseCaseAgainstAction(ctx)
	if err != nil {
		return "", fmt.Errorf("no matching use-case found for execution plan")
	}

	// Validate service capabilities against use case requirements
	if err := g.validateServiceCapabilities(ctx, useCase); err != nil {
		return "", fmt.Errorf("service capabilities validation failed: %w", err)
	}

	// Build PDDL domain content
	var domain strings.Builder

	// Add domain header
	domain.WriteString(fmt.Sprintf("(define (domain %s)\n", g.groundingSpec.Domain))
	domain.WriteString("  (:requirements :strips :typing)\n\n")

	// Add types section
	g.addTypes(&domain, useCase)

	// Add predicates section
	g.addPredicates(&domain, useCase)

	// Add actions section
	g.addActions(&domain, useCase)

	domain.WriteString(")\n")

	return domain.String(), nil
}

func (g *PddlDomainGenerator) addTypes(domain *strings.Builder, useCase *GroundingUseCase) {
	domain.WriteString("  (:types\n")
	// Base types
	domain.WriteString("    service - object\n")

	// Parameter types from grounding
	for paramName := range useCase.Params {
		paramType := g.inferTypeFromParamName(paramName)
		domain.WriteString(fmt.Sprintf("    %s - object\n", paramType))
	}
	domain.WriteString("  )\n\n")
}

func (g *PddlDomainGenerator) addPredicates(domain *strings.Builder, useCase *GroundingUseCase) {
	domain.WriteString("  (:predicates\n")

	// Service state predicates
	domain.WriteString("    (service-validated ?s - service)\n")
	domain.WriteString("    (service-active ?s - service)\n")
	domain.WriteString("    (service-complete ?s - service)\n")

	// Track which parameters are actually used in service inputs
	usedParams := make(map[string]bool)

	// Collect all input parameters from services
	for _, task := range g.executionPlan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) {
			continue
		}

		// Check input properties
		for propName := range task.ExpectedInput.Properties {
			// Normalize the property name to match grounding param style
			normalizedProp := g.normalizePropertyToParam(propName)
			usedParams[normalizedProp] = true
		}
	}

	// Only generate predicates for parameters that are used in services
	for paramName := range useCase.Params {
		if usedParams[paramName] {
			paramType := g.inferTypeFromParamName(paramName)
			domain.WriteString(fmt.Sprintf("    (valid-%s ?%s - %s)\n",
				paramName, paramName, paramType))
		}
	}

	// Task dependencies
	domain.WriteString("    (depends-on ?s1 - service ?s2 - service)\n")

	domain.WriteString("  )\n\n")
}

func (g *PddlDomainGenerator) addActions(domain *strings.Builder, useCase *GroundingUseCase) {
	// Execute service action
	domain.WriteString("  (:action execute-service\n")
	domain.WriteString("   :parameters (?s - service)\n")
	domain.WriteString("   :precondition (and\n")
	domain.WriteString("     (service-validated ?s)\n")
	domain.WriteString("     (service-active ?s)\n")

	// Parameter requirements
	for paramName := range useCase.Params {
		domain.WriteString(fmt.Sprintf("     (valid-%s ?%s)\n",
			paramName, paramName))
	}

	// Dependencies
	domain.WriteString("     (forall (?dep - service)\n")
	domain.WriteString("       (imply (depends-on ?s ?dep)\n")
	domain.WriteString("              (service-complete ?dep)))\n")

	domain.WriteString("   )\n")
	domain.WriteString("   :effect (and\n")
	domain.WriteString("     (service-complete ?s)\n")
	domain.WriteString("     (not (service-active ?s))\n")
	domain.WriteString("   )\n")
	domain.WriteString("  )\n")
}

func (g *PddlDomainGenerator) validateServiceCapabilities(ctx context.Context, useCase *GroundingUseCase) error {
	for _, task := range g.executionPlan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) {
			continue
		}

		// For each required capability in use case
		for _, requiredCap := range useCase.Capabilities {
			capabilityMatched := false

			// Check if any service capability matches
			for _, serviceCap := range task.Capabilities {
				matches, _, err := g.matcher.MatchTexts(ctx, requiredCap, serviceCap, 0.85)
				if err != nil {
					return fmt.Errorf("capability matching failed: %w", err)
				}
				if matches {
					capabilityMatched = true
					break
				}
			}

			if !capabilityMatched {
				return fmt.Errorf("service %s missing required capability: %s",
					task.Service, requiredCap)
			}
		}
	}
	return nil
}

// inferTypeFromParamName infers PDDL type from parameter name
func (g *PddlDomainGenerator) inferTypeFromParamName(paramName string) string {
	// Clean the parameter name
	paramName = strings.ToLower(paramName)

	// Common ID parameters
	if strings.HasSuffix(paramName, "id") {
		// Extract the entity name (e.g., "orderId" -> "order")
		entityName := strings.TrimSuffix(paramName, "id")
		return fmt.Sprintf("%s-id", entityName)
	}

	// Common status parameters
	if strings.HasSuffix(paramName, "status") {
		// Extract the entity name (e.g., "orderStatus" -> "order")
		entityName := strings.TrimSuffix(paramName, "status")
		return fmt.Sprintf("%s-status", entityName)
	}

	// Parameters indicating location
	if strings.Contains(paramName, "location") ||
		strings.Contains(paramName, "address") {
		return "location"
	}

	// Parameters indicating time or date
	if strings.Contains(paramName, "time") ||
		strings.Contains(paramName, "date") {
		return "timestamp"
	}

	// Parameters indicating amounts
	if strings.Contains(paramName, "amount") ||
		strings.Contains(paramName, "price") ||
		strings.Contains(paramName, "cost") {
		return "number"
	}

	// Fallback: use parameter name as type
	return paramName + "-value"
}

// normalizePropertyToParam converts a service property name to match grounding param style
func (g *PddlDomainGenerator) normalizePropertyToParam(propName string) string {
	// Handle common conversions
	switch strings.ToLower(propName) {
	case "orderid":
		return "orderId"
	case "customerid":
		return "customerId"
	case "productid":
		return "productId"
	default:
		return propName
	}
}
