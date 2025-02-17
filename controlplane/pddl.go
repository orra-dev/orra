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

type PddlGenerator struct {
	planAction    string
	executionPlan *ExecutionPlan
	groundingSpec *GroundingSpec
	task0Params   map[string]any
	matcher       SimilarityMatcher
	logger        zerolog.Logger
}

func NewPddlGenerator(action string, plan *ExecutionPlan, spec *GroundingSpec, matcher SimilarityMatcher, logger zerolog.Logger) *PddlGenerator {
	return &PddlGenerator{
		planAction:    action,
		executionPlan: plan,
		groundingSpec: spec,
		matcher:       matcher,
		logger:        logger,
	}
}

func (g *PddlGenerator) ShouldGeneratePddl() bool {
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

func (g *PddlGenerator) findMatchingUseCaseAgainstAction(ctx context.Context) (*GroundingUseCase, error) {
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

func (g *PddlGenerator) planActionMatches(ctx context.Context, useCaseAction string) (bool, error) {
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

func (g *PddlGenerator) normalizeActionPattern(pattern string) string {
	// Remove variable placeholders
	vars := regexp.MustCompile(`\{[^}]+\}`)
	return vars.ReplaceAllString(pattern, "")
}

func (g *PddlGenerator) GenerateDomain(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if !g.ShouldGeneratePddl() {
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

	// Add sections with error handling
	if err := g.addTypes(&domain); err != nil {
		return "", fmt.Errorf("failed to generate types: %w", err)
	}

	if err := g.addPredicates(&domain); err != nil {
		return "", fmt.Errorf("failed to generate predicates: %w", err)
	}

	if err := g.addActions(&domain); err != nil {
		return "", fmt.Errorf("failed to generate actions: %w", err)
	}

	domain.WriteString(")\n")

	return domain.String(), nil
}

func (g *PddlGenerator) addTypes(domain *strings.Builder) error {
	params, err := g.extractTask0Parameters()
	if err != nil {
		return err
	}

	domain.WriteString("  (:types\n")
	// Base types
	domain.WriteString("    service - object\n")

	// Parameter types from Task0
	addedTypes := make(map[string]bool)
	for paramName := range params {
		paramType := g.inferTypeFromParamName(paramName)
		if !addedTypes[paramType] {
			domain.WriteString(fmt.Sprintf("    %s - object\n", paramType))
			addedTypes[paramType] = true
		}
	}
	domain.WriteString("  )\n\n")

	return nil
}

func (g *PddlGenerator) addPredicates(domain *strings.Builder) error {
	params, err := g.extractTask0Parameters()
	if err != nil {
		return err
	}

	domain.WriteString("  (:predicates\n")

	// Service state predicates
	domain.WriteString("    (service-validated ?s - service)\n")
	domain.WriteString("    (service-active ?s - service)\n")
	domain.WriteString("    (service-complete ?s - service)\n")

	// Parameter validity predicates from Task0
	for paramName := range params {
		paramType := g.inferTypeFromParamName(paramName)
		domain.WriteString(fmt.Sprintf("    (valid-%s ?%s - %s)\n",
			paramName, paramName, paramType))
	}

	// Task dependencies
	domain.WriteString("    (depends-on ?s1 - service ?s2 - service)\n")

	domain.WriteString("  )\n\n")
	return nil
}

func (g *PddlGenerator) addActions(domain *strings.Builder) error {
	params, err := g.extractTask0Parameters()
	if err != nil {
		return err
	}

	domain.WriteString("  (:action execute-service\n")

	// Add parameters section with service and all Task0 parameters
	domain.WriteString("   :parameters (?s - service")
	for paramName := range params {
		paramType := g.inferTypeFromParamName(paramName)
		domain.WriteString(fmt.Sprintf(" ?%s - %s", paramName, paramType))
	}
	domain.WriteString(")\n")

	domain.WriteString("   :precondition (and\n")
	domain.WriteString("     (service-validated ?s)\n")
	domain.WriteString("     (service-active ?s)\n")

	// Parameter requirements from Task0
	for paramName := range params {
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

	return nil
}

// validateServiceCapabilities checks if any service in the plan can fulfill each required capability
func (g *PddlGenerator) validateServiceCapabilities(ctx context.Context, useCase *GroundingUseCase) error {
	// Collect all service capabilities
	var allServiceCapabilities []string
	for _, task := range g.executionPlan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) {
			continue
		}
		allServiceCapabilities = append(allServiceCapabilities, task.Capabilities...)
	}

	// Track matched capabilities to ensure all are covered
	matchedCapabilities := make(map[string]bool)

	// For each required capability, check if any service can fulfill it
	for _, requiredCap := range useCase.Capabilities {
		capabilityMatched := false

		// Check against all service capabilities
		for _, serviceCap := range allServiceCapabilities {
			matches, score, err := g.matcher.MatchTexts(ctx, requiredCap, serviceCap, 0.75)
			if err != nil {
				return fmt.Errorf("capability matching failed: %w", err)
			}

			g.logger.Debug().
				Str("requiredCapability", requiredCap).
				Str("serviceCapability", serviceCap).
				Float64("score", score).
				Bool("matches", matches).
				Msg("Matching capabilities")

			if matches {
				capabilityMatched = true
				matchedCapabilities[requiredCap] = true
				break
			}
		}

		if !capabilityMatched {
			return fmt.Errorf("no service found with required capability: %s", requiredCap)
		}
	}

	// Ensure all required capabilities were matched
	for _, requiredCap := range useCase.Capabilities {
		if !matchedCapabilities[requiredCap] {
			return fmt.Errorf("missing required capability: %s", requiredCap)
		}
	}

	return nil
}

// New method to extract and validate Task0 parameters
func (g *PddlGenerator) extractTask0Parameters() (map[string]any, error) {
	if g.task0Params != nil {
		return g.task0Params, nil
	}

	// Extract Task0 and verify it exists
	taskZero, _ := g.callingPlanMinusTaskZero(g.executionPlan)
	if taskZero == nil {
		return nil, fmt.Errorf("task zero not found in execution plan")
	}

	g.task0Params = taskZero.Input
	return g.task0Params, nil
}

func (g *PddlGenerator) GenerateProblem(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if !g.ShouldGeneratePddl() {
		return "", fmt.Errorf("no domain grounding available for PDDL generation")
	}

	// Extract Task0 and service tasks
	taskZero, servicePlan := g.callingPlanMinusTaskZero(g.executionPlan)
	if taskZero == nil {
		return "", fmt.Errorf("task zero not found in execution plan")
	}

	// Build PDDL problem content
	var problem strings.Builder

	// Add problem header - use domain name from spec
	domainName := g.groundingSpec.Domain
	problem.WriteString(fmt.Sprintf("(define (problem p-%s)\n", g.executionPlan.ProjectID))
	problem.WriteString(fmt.Sprintf("  (:domain %s)\n\n", domainName))

	// Add objects section
	if err := g.addProblemObjects(&problem, taskZero, servicePlan.Tasks); err != nil {
		return "", fmt.Errorf("failed to generate objects: %w", err)
	}

	// Add init section
	if err := g.addProblemInit(&problem, taskZero, servicePlan.Tasks); err != nil {
		return "", fmt.Errorf("failed to generate init state: %w", err)
	}

	// Add goal section
	g.addProblemGoal(&problem, servicePlan.Tasks)

	problem.WriteString(")\n")

	return problem.String(), nil
}

func (g *PddlGenerator) callingPlanMinusTaskZero(plan *ExecutionPlan) (*SubTask, *ExecutionPlan) {
	var taskZero *SubTask
	var serviceTasks []*SubTask

	for _, task := range plan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) {
			taskZero = task
			continue
		}
		serviceTasks = append(serviceTasks, task)
	}

	return taskZero, &ExecutionPlan{
		ProjectID: plan.ProjectID,
		Tasks:     serviceTasks,
	}
}

func (g *PddlGenerator) addProblemObjects(problem *strings.Builder, taskZero *SubTask, tasks []*SubTask) error {
	problem.WriteString("  (:objects\n")

	// Add service objects
	problem.WriteString("    ; Services\n")
	for _, task := range tasks {
		problem.WriteString(fmt.Sprintf("    %s - service\n", task.ID))
	}

	// Add parameter objects from Task0 input
	problem.WriteString("\n    ; Parameters\n")
	for paramName, paramValue := range taskZero.Input {
		paramType := g.inferTypeFromParamName(paramName)

		// Handle different value types
		var objName string
		switch v := paramValue.(type) {
		case string:
			objName = v
		case float64:
			objName = fmt.Sprintf("n%v", v)
		default:
			// Skip complex types
			continue
		}

		// Clean object name for PDDL
		objName = strings.ReplaceAll(objName, "-", "_")
		objName = strings.ReplaceAll(objName, " ", "_")

		problem.WriteString(fmt.Sprintf("    %s - %s\n", objName, paramType))
	}

	problem.WriteString("  )\n\n")
	return nil
}

func (g *PddlGenerator) addProblemInit(problem *strings.Builder, taskZero *SubTask, tasks []*SubTask) error {
	problem.WriteString("  (:init\n")

	// Add service states
	problem.WriteString("    ; Initial service states\n")
	for _, task := range tasks {
		problem.WriteString(fmt.Sprintf("    (service-validated %s)\n", task.ID))
		problem.WriteString(fmt.Sprintf("    (service-active %s)\n", task.ID))
	}

	// Add parameter validations from Task0
	problem.WriteString("\n    ; Parameter validations\n")
	for paramName, paramValue := range taskZero.Input {
		// Handle different value types
		var objName string
		switch v := paramValue.(type) {
		case string:
			objName = v
		case float64:
			objName = fmt.Sprintf("n%v", v)
		default:
			// Skip complex types
			continue
		}

		// Clean object name for PDDL
		objName = strings.ReplaceAll(objName, "-", "_")
		objName = strings.ReplaceAll(objName, " ", "_")

		problem.WriteString(fmt.Sprintf("    (valid-%s %s)\n", paramName, objName))
	}

	// Add dependencies based on data flow
	problem.WriteString("\n    ; Task dependencies\n")
	for _, task := range tasks {
		deps := task.extractDependencies()
		for depTaskID := range deps {
			if strings.EqualFold(depTaskID, TaskZero) {
				continue // Skip Task0 dependencies in PDDL
			}
			problem.WriteString(fmt.Sprintf("    (depends-on %s %s)\n", task.ID, depTaskID))
		}
	}

	problem.WriteString("  )\n\n")
	return nil
}

func (g *PddlGenerator) addProblemGoal(problem *strings.Builder, tasks []*SubTask) {
	problem.WriteString("  (:goal\n")
	problem.WriteString("    (and\n")

	// Goal is to complete all services
	for _, task := range tasks {
		problem.WriteString(fmt.Sprintf("      (service-complete %s)\n", task.ID))
	}

	problem.WriteString("    )\n")
	problem.WriteString("  )\n")
}

// inferTypeFromParamName infers PDDL type from parameter name
func (g *PddlGenerator) inferTypeFromParamName(paramName string) string {
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
