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

// PDDLDomainGenerator handles converting grounding specs to PDDL
type PDDLDomainGenerator struct {
	planAction    string
	executionPlan *ExecutionPlan
	groundingSpec *GroundingSpec
}

func NewPDDLDomainGenerator(action string, plan *ExecutionPlan, spec *GroundingSpec) *PDDLDomainGenerator {
	return &PDDLDomainGenerator{
		planAction:    action,
		executionPlan: plan,
		groundingSpec: spec,
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
func (g *PDDLDomainGenerator) GenerateDomain() (string, error) {
	if !g.ShouldGenerateDomain() {
		return "", fmt.Errorf("no domain grounding available for PDDL generation")
	}

	// Find matching use-case based on execution plan
	useCase := g.findMatchingUseCase()
	if useCase == nil {
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

func (g *PDDLDomainGenerator) findMatchingUseCase() *GroundingUseCase {
	for _, useCase := range g.groundingSpec.UseCases {
		if g.planActionMatches(useCase.Action) {
			return &useCase
		}
	}

	return nil
}

func (g *PDDLDomainGenerator) planActionMatches(useCaseAction string) bool {
	// Convert use-case action pattern to regex
	// e.g. "Handle customer inquiry about {orderId}"
	// -> "Handle customer inquiry about .*"
	pattern := regexp.QuoteMeta(useCaseAction)
	pattern = strings.ReplaceAll(pattern, `\{[^}]+\}`, ".*")

	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}

	return re.MatchString(g.planAction)
}

func (g *PDDLDomainGenerator) addTypes(_ *strings.Builder, _ *GroundingUseCase) {}

func (g *PDDLDomainGenerator) addPredicates(_ *strings.Builder, _ *GroundingUseCase) {}

func (g *PDDLDomainGenerator) addActions(_ *strings.Builder, _ *GroundingUseCase) {}
