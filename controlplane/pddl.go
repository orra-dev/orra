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

func (g *PDDLDomainGenerator) addTypes(_ *strings.Builder, _ *GroundingUseCase) {}

func (g *PDDLDomainGenerator) addPredicates(_ *strings.Builder, _ *GroundingUseCase) {}

func (g *PDDLDomainGenerator) addActions(_ *strings.Builder, _ *GroundingUseCase) {}
