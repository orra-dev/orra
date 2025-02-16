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
	"unicode"

	"github.com/rs/zerolog"
)

// PddlDomainGenerator handles converting grounding specs to PDDL
type PddlDomainGenerator struct {
	planAction    string
	executionPlan *ExecutionPlan
	groundingSpec *GroundingSpec
	matcher       *Matcher
	logger        zerolog.Logger
}

type PddlPredicate struct {
	name       string
	parameters []PddlPredicateParam
}

// PddlPredicateParam represents a parameter in a PDDL predicate
type PddlPredicateParam struct {
	name string
	typ  string
}

// PddlAction represents a PDDL action with its components
type PddlAction struct {
	name       string
	parameters []PddlActionParam
	precond    []string // List of predicates that must be true before action
	effects    []string // List of predicates that become true after action
}

type PddlActionParam struct {
	name string
	typ  string
}

func NewPDDLDomainGenerator(action string, plan *ExecutionPlan, spec *GroundingSpec, matcher *Matcher, logger zerolog.Logger) *PddlDomainGenerator {
	return &PddlDomainGenerator{
		planAction:    action,
		executionPlan: plan,
		groundingSpec: spec,
		matcher:       matcher,
		logger:        logger,
	}
}

// ShouldGenerateDomain checks if we should proceed with PDDL generation
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

// GenerateDomain creates PDDL domain file content from matching use-case
func (g *PddlDomainGenerator) GenerateDomain(ctx context.Context) (string, error) {
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

func (g *PddlDomainGenerator) addTypes(domain *strings.Builder, useCase *GroundingUseCase) {
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

// inferParentType determines the parent type based on the type name
func (g *PddlDomainGenerator) inferParentType(typeName string) string {
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

func (g *PddlDomainGenerator) inferTypeFromParam(param interface{}, subtasks []*SubTask) string {
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

func (g *PddlDomainGenerator) addPredicates(domain *strings.Builder, useCase *GroundingUseCase) {
	predicates := make(map[string]PddlPredicate)

	// Helper to add a predicate while avoiding duplicates
	addPredicate := func(pred PddlPredicate) {
		if existing, exists := predicates[pred.name]; exists {
			g.logger.Debug().
				Str("predicate", pred.name).
				Interface("old_params", existing.parameters).
				Interface("new_params", pred.parameters).
				Msg("Overwriting existing predicate")
		}
		predicates[pred.name] = pred
		g.logger.Debug().
			Str("predicate", pred.name).
			Interface("parameters", pred.parameters).
			Msg("Added predicate")
	}

	// 1. Add status predicates from capabilities
	if useCase.Capabilities != nil {
		for _, capability := range useCase.Capabilities {
			switch {
			case strings.Contains(strings.ToLower(capability), "status"):
				// e.g., "Check order status" -> (has-status ?order - order-id ?status - order-status)
				addPredicate(PddlPredicate{
					name: "has-status",
					parameters: []PddlPredicateParam{
						{name: "?obj", typ: g.inferObjectType(capability)},
						{name: "?status", typ: g.inferStatusType(capability)},
					},
				})
			case strings.Contains(strings.ToLower(capability), "location"):
				// e.g., "Check store location" -> (at-location ?obj - object ?loc - location)
				addPredicate(PddlPredicate{
					name: "at-location",
					parameters: []PddlPredicateParam{
						{name: "?obj", typ: g.inferObjectType(capability)},
						{name: "?location", typ: "location"},
					},
				})
			}
		}
	}

	// 2. Add predicates from service schemas
	for _, task := range g.executionPlan.Tasks {
		if strings.EqualFold(task.ID, TaskZero) {
			continue
		}

		// Process input properties for relationships
		if task.ExpectedInput.Properties != nil {
			for propName, propSpec := range task.ExpectedInput.Properties {
				pddlType := g.inferDetailedType(propSpec.Type, propName)

				// Add existence predicate for IDs
				if strings.HasSuffix(pddlType, "-id") {
					objType := strings.TrimSuffix(pddlType, "-id")
					addPredicate(PddlPredicate{
						name: fmt.Sprintf("exists-%s", objType),
						parameters: []PddlPredicateParam{
							{name: "?id", typ: pddlType},
						},
					})
				}

				// Add boolean properties as predicates
				if propSpec.Type == "boolean" {
					normalizedName := normalizePropName(propName)
					addPredicate(PddlPredicate{
						name: normalizedName,
						parameters: []PddlPredicateParam{
							{name: "?obj", typ: g.inferObjectTypeFromTask(task)},
						},
					})
				}
			}
		}

		// Process output properties for additional states
		if task.ExpectedOutput.Properties != nil {
			for propName, propSpec := range task.ExpectedOutput.Properties {
				if strings.Contains(strings.ToLower(propName), "status") {
					objType := g.inferObjectTypeFromTask(task)
					statusType := g.inferDetailedType(propSpec.Type, propName)
					addPredicate(PddlPredicate{
						name: "has-status",
						parameters: []PddlPredicateParam{
							{name: "?obj", typ: objType + "-id"},
							{name: "?status", typ: statusType},
						},
					})
				}
			}
		}
	}

	// Write predicates section if we have predicates to write
	if len(predicates) > 0 {
		domain.WriteString("  (:predicates\n")

		// Sort predicates for consistent output
		sortedPredicates := make([]PddlPredicate, 0, len(predicates))
		for _, pred := range predicates {
			sortedPredicates = append(sortedPredicates, pred)
		}
		sort.Slice(sortedPredicates, func(i, j int) bool {
			return sortedPredicates[i].name < sortedPredicates[j].name
		})

		// Write each predicate
		for _, pred := range sortedPredicates {
			params := make([]string, len(pred.parameters))
			for i, param := range pred.parameters {
				params[i] = fmt.Sprintf("%s - %s", param.name, param.typ)
			}
			domain.WriteString(fmt.Sprintf("    (%s %s)\n",
				pred.name,
				strings.Join(params, " ")))
		}

		domain.WriteString("  )\n\n")
	}
}

func (g *PddlDomainGenerator) inferObjectType(capability string) string {
	lower := strings.ToLower(capability)

	// For location-related capabilities, keep the object type generic
	if strings.Contains(lower, "location") {
		return "object" // Generic type for location predicates
	}

	// For other capabilities, infer specific types
	switch {
	case strings.Contains(lower, "order"):
		return "order-id"
	case strings.Contains(lower, "product"):
		return "product-id"
	case strings.Contains(lower, "payment"):
		return "payment-id"
	default:
		return "object"
	}
}

func (g *PddlDomainGenerator) inferStatusType(capability string) string {
	lower := strings.ToLower(capability)
	switch {
	case strings.Contains(lower, "order"):
		return "order-status"
	case strings.Contains(lower, "payment"):
		return "payment-status"
	case strings.Contains(lower, "shipping"):
		return "shipping-status"
	default:
		return "status"
	}
}

func (g *PddlDomainGenerator) inferObjectTypeFromTask(task *SubTask) string {
	service := strings.ToLower(task.Service)
	switch {
	case strings.Contains(service, "order"):
		return "order"
	case strings.Contains(service, "payment"):
		return "payment"
	case strings.Contains(service, "product"):
		return "product"
	default:
		return "object"
	}
}

func normalizePropName(name string) string {
	// Convert camelCase to kebab-case for PDDL
	var result strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune('-')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

func (g *PddlDomainGenerator) addActions(domain *strings.Builder, useCase *GroundingUseCase) {
	// Process each task in the execution plan as a potential action
	for _, task := range g.executionPlan.Tasks {
		// Skip task zero as it's not a real action
		if strings.EqualFold(task.ID, TaskZero) {
			continue
		}

		action := g.createActionFromTask(task, useCase)

		// Write the action definition
		domain.WriteString(fmt.Sprintf("  (:action %s\n", action.name))

		// Write parameters
		if len(action.parameters) > 0 {
			domain.WriteString("   :parameters (")
			params := make([]string, len(action.parameters))
			for i, param := range action.parameters {
				params[i] = fmt.Sprintf("%s - %s", param.name, param.typ)
			}
			domain.WriteString(strings.Join(params, " "))
			domain.WriteString(")\n")
		}

		// Write preconditions
		if len(action.precond) > 0 {
			domain.WriteString("   :precondition (and\n")
			for _, pred := range action.precond {
				domain.WriteString(fmt.Sprintf("    %s\n", pred))
			}
			domain.WriteString("   )\n")
		}

		// Write effects
		if len(action.effects) > 0 {
			domain.WriteString("   :effect (and\n")
			for _, effect := range action.effects {
				domain.WriteString(fmt.Sprintf("    %s\n", effect))
			}
			domain.WriteString("   )\n")
		}

		domain.WriteString("  )\n\n")
	}
}

func (g *PddlDomainGenerator) createActionFromTask(task *SubTask, useCase *GroundingUseCase) PddlAction {
	action := PddlAction{
		name:       g.normalizeActionName(task, useCase),
		parameters: make([]PddlActionParam, 0),
		precond:    make([]string, 0),
		effects:    make([]string, 0),
	}

	// Track parameters we've already added
	seenParams := make(map[string]bool)

	// Process input properties for parameters and preconditions
	if task.ExpectedInput.Properties != nil {
		for propName, propSpec := range task.ExpectedInput.Properties {
			pddlType := g.inferDetailedType(propSpec.Type, propName)

			// Add parameter
			paramName := "?" + g.normalizeParamName(propName)
			if !seenParams[paramName] {
				action.parameters = append(action.parameters, PddlActionParam{
					name: paramName,
					typ:  pddlType,
				})
				seenParams[paramName] = true
			}

			// Add existence precondition for IDs
			if strings.HasSuffix(pddlType, "-id") {
				objType := strings.TrimSuffix(pddlType, "-id")
				action.precond = append(action.precond,
					fmt.Sprintf("(exists-%s %s)", objType, paramName))
			}
		}
	}

	// Process output properties for effects
	if task.ExpectedOutput.Properties != nil {
		for propName, propSpec := range task.ExpectedOutput.Properties {
			if strings.Contains(strings.ToLower(propName), "status") {
				// Add status change effect
				objParam := g.findRelatedObjectParam(action.parameters)
				statusType := g.inferDetailedType(propSpec.Type, propName)
				action.parameters = append(action.parameters, PddlActionParam{
					name: "?new-status",
					typ:  statusType,
				})
				action.effects = append(action.effects,
					fmt.Sprintf("(has-status %s ?new-status)", objParam))
			}
		}
	}

	// Add location effects if capability mentions location
	for _, capability := range useCase.Capabilities {
		if strings.Contains(strings.ToLower(capability), "location") {
			locationParam := "?loc"
			if !seenParams[locationParam] {
				action.parameters = append(action.parameters, PddlActionParam{
					name: locationParam,
					typ:  "location",
				})
				seenParams[locationParam] = true
			}
			objParam := g.findRelatedObjectParam(action.parameters)
			action.effects = append(action.effects,
				fmt.Sprintf("(at-location %s %s)", objParam, locationParam))
		}
	}

	return action
}

func (g *PddlDomainGenerator) normalizeActionName(task *SubTask, useCase *GroundingUseCase) string {
	// First try to extract verb from use case action
	action := strings.ToLower(useCase.Action)
	words := strings.Fields(action)

	if len(words) > 0 {
		// First word is usually the verb (e.g., "Process order", "Ship product")
		verb := words[0]

		// Get the domain object being operated on
		domain := g.inferDomainObject(task)

		// Combine verb and domain object
		return fmt.Sprintf("%s-%s", verb, domain)
	}

	// Fallback: If we can't extract from action, make a safe conversion of the service name
	sanitized := strings.ToLower(task.Service)
	// Remove common suffixes
	sanitized = strings.TrimSuffix(sanitized, "_service")
	sanitized = strings.TrimSuffix(sanitized, "service")
	sanitized = strings.TrimSuffix(sanitized, "-service")
	// Replace any remaining non-alphanumeric with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	sanitized = reg.ReplaceAllString(sanitized, "-")
	// Trim hyphens from ends
	sanitized = strings.Trim(sanitized, "-")

	if sanitized == "" {
		// Ultimate fallback for completely unusual service names
		return fmt.Sprintf("action-%s", task.ID)
	}

	return fmt.Sprintf("execute-%s", sanitized)
}

func (g *PddlDomainGenerator) inferDomainObject(task *SubTask) string {
	// Try to infer the main domain object this task operates on

	// First check input properties for ID types
	if task.ExpectedInput.Properties != nil {
		for propName := range task.ExpectedInput.Properties {
			if strings.HasSuffix(strings.ToLower(propName), "id") {
				// Extract domain object from property name
				// e.g., "orderId" -> "order", "productId" -> "product"
				obj := strings.ToLower(strings.TrimSuffix(propName, "Id"))
				if obj != "" {
					return obj
				}
			}
		}
	}

	// Check service name for domain hints
	serviceLower := strings.ToLower(task.Service)
	commonDomains := []string{"order", "payment", "product", "customer", "inventory", "shipping"}
	for _, domain := range commonDomains {
		if strings.Contains(serviceLower, domain) {
			return domain
		}
	}

	// Fallback to a generic name
	return "operation"
}

func (g *PddlDomainGenerator) normalizeParamName(name string) string {
	// First handle special case for ID parameters
	nameLower := strings.ToLower(name)
	if strings.HasSuffix(nameLower, "id") {
		// Find the ID part in original name to preserve boundary
		idIndex := strings.LastIndex(strings.ToLower(name), "id")
		if idIndex > 0 {
			prefix := strings.ToLower(name[:idIndex])
			// Remove any existing separators
			prefix = strings.Trim(prefix, "_-")
			if prefix != "" {
				return prefix + "-id"
			}
		}
		return "id"
	}

	// For other parameters, convert camelCase to kebab-case
	var result strings.Builder
	var lastUpper bool

	// Convert the first character to lowercase
	runes := []rune(name)
	for i, r := range runes {
		isUpper := unicode.IsUpper(r)

		// Add hyphen if:
		// 1. Not the first character
		// 2. Current char is uppercase and previous char wasn't uppercase (camelCase boundary)
		// 3. Or next char is lowercase (end of acronym)
		if i > 0 && isUpper &&
			(!lastUpper || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			result.WriteRune('-')
		}

		result.WriteRune(unicode.ToLower(r))
		lastUpper = isUpper
	}

	// Clean up any duplicate hyphens and trim
	normalized := strings.Trim(result.String(), "-")
	normalized = strings.ReplaceAll(normalized, "--", "-")

	if normalized == "" {
		return "param"
	}

	return normalized
}

func (g *PddlDomainGenerator) findRelatedObjectParam(params []PddlActionParam) string {
	// Find the main object parameter (usually an ID) for this action
	for _, param := range params {
		if strings.HasSuffix(param.typ, "-id") {
			return param.name
		}
	}
	return "?obj" // Fallback to generic object
}

// inferDetailedType provides more specific type inference based on property name and context
func (g *PddlDomainGenerator) inferDetailedType(jsonType, propName string) string {
	switch jsonType {
	case "string":
		return g.inferStringType(propName)
	case "number", "integer":
		return g.inferNumberType(propName)
	case "boolean":
		return "boolean"
	case "array":
		return "collection"
	default:
		return "object"
	}
}

func (g *PddlDomainGenerator) inferStringType(propName string) string {
	propNameLower := strings.ToLower(propName)

	// Map of suffixes to their base types
	suffixTypes := map[string]string{
		"id":       "id",
		"location": "location",
		"status":   "status",
	}

	for suffix, baseType := range suffixTypes {
		if strings.HasSuffix(propNameLower, suffix) {
			suffixIndex := strings.LastIndex(strings.ToLower(propName), suffix)
			if suffixIndex > 0 {
				prefix := strings.ToLower(propName[:suffixIndex])
				prefix = strings.Trim(prefix, "_-")
				if prefix != "" {
					return fmt.Sprintf("%s-%s", prefix, baseType)
				}
			}
			return baseType
		}
	}

	return "object"
}

func (g *PddlDomainGenerator) inferNumberType(propName string) string {
	propNameLower := strings.ToLower(propName)
	if strings.Contains(propNameLower, "amount") ||
		strings.Contains(propNameLower, "price") ||
		strings.Contains(propNameLower, "cost") {
		return "monetary-value"
	}
	return "number"
}
