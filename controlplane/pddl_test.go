/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

// FakeMatcher implements both Matcher and Embedder interfaces for testing
type FakeMatcher struct {
	// Configured responses for testing
	matchResponse   bool
	matchScore      float64
	matchError      error
	embeddings      []float32
	embeddingsError error
}

func NewFakeMatcher() *FakeMatcher {
	return &FakeMatcher{
		matchResponse: true,
		matchScore:    0.9,
		embeddings:    []float32{0.1, 0.2, 0.3},
	}
}

func (f *FakeMatcher) MatchTexts(_ context.Context, _, _ string, _ float64) (bool, float64, error) {
	return f.matchResponse, f.matchScore, f.matchError
}

func (f *FakeMatcher) CreateEmbeddings(_ context.Context, _ string) ([]float32, error) {
	return f.embeddings, f.embeddingsError
}

func TestPddlDomainGenerator_ShouldGenerateDomain(t *testing.T) {
	tests := []struct {
		name          string
		groundingSpec *GroundingSpec
		groundingID   string
		want          bool
	}{
		{
			name:          "no grounding spec",
			groundingSpec: nil,
			groundingID:   "",
			want:          false,
		},
		{
			name: "has grounding spec but no ID",
			groundingSpec: &GroundingSpec{
				Name: "test-grounding",
			},
			groundingID: "",
			want:        false,
		},
		{
			name: "has both grounding spec and ID",
			groundingSpec: &GroundingSpec{
				Name: "test-grounding",
			},
			groundingID: "test-grounding",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &ExecutionPlan{
				GroundingID: tt.groundingID,
			}
			generator := NewPddlDomainGenerator(
				"test-action",
				plan,
				tt.groundingSpec,
				NewFakeMatcher(),
				zerolog.Nop(),
			)

			got := generator.ShouldGenerateDomain()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPddlDomainGenerator_GenerateDomain(t *testing.T) {
	// Test case: e-commerce customer support
	useCase := GroundingUseCase{
		Action: "Process refund for order {orderId}",
		Params: map[string]string{
			"orderId": "ORD123", // Only include params that match service requirements
		},
		Capabilities: []string{
			"Verify refund eligibility",
			"Process payment refund",
		},
	}

	groundingSpec := &GroundingSpec{
		Name:     "customer-support",
		Domain:   "e-commerce-customer-support",
		Version:  "1.0",
		UseCases: []GroundingUseCase{useCase},
	}

	executionPlan := &ExecutionPlan{
		GroundingID: "customer-support",
		Tasks: []*SubTask{
			{
				ID:      "task1",
				Service: "refund-service",
				Capabilities: []string{
					"A service that handles refund processing including eligibility checks and payment refunds",
				},
				ExpectedInput: Spec{
					Properties: map[string]Spec{
						"orderId": {Type: "string"},
					},
				},
				ExpectedOutput: Spec{
					Properties: map[string]Spec{
						"refundStatus": {Type: "string"},
					},
				},
			},
		},
	}

	matcher := NewFakeMatcher()
	matcher.matchResponse = true
	matcher.matchScore = 0.9

	generator := NewPddlDomainGenerator(
		"Process refund for order ORD123",
		executionPlan,
		groundingSpec,
		matcher,
		zerolog.Nop(),
	)

	domain, err := generator.GenerateDomain(context.Background())
	assert.NoError(t, err)
	assert.NotEmpty(t, domain)

	// Verify domain contains expected elements
	assert.Contains(t, domain, "(define (domain e-commerce-customer-support)")
	assert.Contains(t, domain, "(:requirements :strips :typing)")

	// Verify types - should only include types for service inputs
	assert.Contains(t, domain, "order-id - object")
	assert.NotContains(t, domain, "customer-id - object", "Should not include types for unused params")

	// Verify predicates - should only include predicates for service inputs
	assert.Contains(t, domain, "(service-validated ?s - service)")
	assert.Contains(t, domain, "(valid-orderId ?orderId - order-id)")
	assert.NotContains(t, domain, "valid-customerId", "Should not include predicates for unused params")

	// Verify action
	assert.Contains(t, domain, "(:action execute-service")
}

func TestPddlDomainGenerator_ValidateServiceCapabilities(t *testing.T) {
	useCase := GroundingUseCase{
		Capabilities: []string{
			"Verify refund eligibility",
			"Process payment refund",
		},
	}

	tests := []struct {
		name        string
		matchResult bool
		tasks       []*SubTask
		wantErr     bool
	}{
		{
			name:        "matching capabilities",
			matchResult: true,
			tasks: []*SubTask{
				{
					ID:      "task1",
					Service: "refund-service",
					Capabilities: []string{
						"A service that handles refund processing and verification",
					},
				},
			},
			wantErr: false,
		},
		{
			name:        "missing capability",
			matchResult: false,
			tasks: []*SubTask{
				{
					ID:      "task1",
					Service: "refund-service",
					Capabilities: []string{
						"A service that only processes payments",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewFakeMatcher()
			matcher.matchResponse = tt.matchResult

			generator := NewPddlDomainGenerator(
				"test-action",
				&ExecutionPlan{Tasks: tt.tasks},
				&GroundingSpec{},
				matcher,
				zerolog.Nop(),
			)

			err := generator.validateServiceCapabilities(context.Background(), &useCase)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "no service found with required capability")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGeneratedPddlSyntax(t *testing.T) {
	useCase := GroundingUseCase{
		Action: "Process refund for order {orderId}",
		Params: map[string]string{
			"orderId":    "ORD123",
			"customerId": "CUST456",
		},
		Capabilities: []string{"Process refund"},
	}

	groundingSpec := &GroundingSpec{
		Name:     "test-grounding",
		Domain:   "test-domain",
		UseCases: []GroundingUseCase{useCase},
	}

	executionPlan := &ExecutionPlan{
		GroundingID: "test-grounding",
		Tasks: []*SubTask{
			{
				ID:           "task1",
				Service:      "refund-service",
				Capabilities: []string{"A refund processing service"},
				ExpectedInput: Spec{
					Properties: map[string]Spec{
						"orderId": {Type: "string"},
					},
				},
				ExpectedOutput: Spec{
					Properties: map[string]Spec{
						"refundStatus": {Type: "string"},
					},
				},
			},
		},
	}

	matcher := NewFakeMatcher()
	matcher.matchResponse = true
	matcher.matchScore = 0.9

	generator := NewPddlDomainGenerator(
		"Process refund for order ORD123",
		executionPlan,
		groundingSpec,
		matcher,
		zerolog.Nop(),
	)

	domain, err := generator.GenerateDomain(context.Background())
	assert.NoError(t, err)

	t.Logf("Generated PDDL:\n%s", domain)

	// Split lines and filter out empty ones
	var nonEmptyLines []string
	for _, line := range strings.Split(domain, "\n") {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	// Check domain definition exists
	hasDomainDef := false
	for _, line := range nonEmptyLines {
		if strings.Contains(line, "(define (domain") {
			hasDomainDef = true
			break
		}
	}
	assert.True(t, hasDomainDef, "Domain definition should exist")

	// Check required sections exist
	hasTypes := false
	hasPredicates := false
	hasActions := false

	for _, line := range nonEmptyLines {
		trimmedLine := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmedLine, "(:types"):
			hasTypes = true
		case strings.HasPrefix(trimmedLine, "(:predicates"):
			hasPredicates = true
		case strings.HasPrefix(trimmedLine, "(:action"):
			hasActions = true
		}
	}

	assert.True(t, hasTypes, "Domain should contain types section")
	assert.True(t, hasPredicates, "Domain should contain predicates section")
	assert.True(t, hasActions, "Domain should contain at least one action")

	// Validate entire structure
	assert.True(t, strings.HasPrefix(strings.TrimSpace(nonEmptyLines[0]), "(define"), "Should start with define")
	lastLine := strings.TrimSpace(nonEmptyLines[len(nonEmptyLines)-1])
	assert.True(t, lastLine == ")", "Should end with closing parenthesis")

	// Count parentheses
	openCount := strings.Count(domain, "(")
	closeCount := strings.Count(domain, ")")
	assert.Equal(t, openCount, closeCount, "Parentheses should be balanced")

	// Additional structure checks
	var foundActionParams, foundPrecondition, foundEffect bool
	for _, line := range nonEmptyLines {
		trimmedLine := strings.TrimSpace(line)
		switch {
		case strings.Contains(trimmedLine, ":parameters"):
			foundActionParams = true
		case strings.Contains(trimmedLine, ":precondition"):
			foundPrecondition = true
		case strings.Contains(trimmedLine, ":effect"):
			foundEffect = true
		}
	}

	assert.True(t, foundActionParams, "Action should have parameters")
	assert.True(t, foundPrecondition, "Action should have preconditions")
	assert.True(t, foundEffect, "Action should have effects")
}
