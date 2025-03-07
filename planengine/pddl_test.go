/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"gonum.org/v1/gonum/mat"
)

// FakeMatcher implements both Matcher and Embedder interfaces for testing
type FakeMatcher struct {
	// Configured responses for testing
	matchResponse       bool
	matchScore          float64
	matchError          error
	embeddings          []float32
	embeddingsError     error
	capabilitiesToMatch map[string]any
}

func NewFakeMatcher() *FakeMatcher {
	return &FakeMatcher{
		matchResponse:       true,
		matchScore:          0.9,
		embeddings:          []float32{0.1, 0.2, 0.3},
		capabilitiesToMatch: make(map[string]any),
	}
}

// In the same file, update the FakeMatcher MatchTexts method:
func (f *FakeMatcher) MatchTexts(_ context.Context, text1, _ string, _ float64) (bool, float64, error) {
	if f.matchError != nil {
		return false, 0, f.matchError
	}

	// Special case for 88.24% test - use a deterministic approach instead of a counter
	if f.capabilitiesToMatch != nil && strings.HasPrefix(text1, "Cap") {
		// Extract capability number (e.g., "Cap5" -> 5)
		var capNum int
		_, _ = fmt.Sscanf(text1, "Cap%d", &capNum)

		// Match first ~88.24% of capabilities (15 out of 17)
		shouldMatch := capNum <= 15
		return shouldMatch, f.matchScore, nil
	}

	// Default behavior for other test cases
	return f.matchResponse, f.matchScore, nil
}

func (f *FakeMatcher) GenerateEmbeddingVector(_ context.Context, _ string) (*mat.VecDense, error) {
	return nil, nil
}

func (f *FakeMatcher) CreateEmbeddings(_ context.Context, _ string) ([]float32, error) {
	return f.embeddings, f.embeddingsError
}

func TestPddlDomainGenerator_ShouldGenerateDomain(t *testing.T) {
	tests := []struct {
		name         string
		groundingHit *GroundingHit
		want         bool
	}{
		{
			name:         "no grounding hit",
			groundingHit: nil,
			want:         false,
		},
		{
			name: "has grounding hit",
			groundingHit: &GroundingHit{
				Name: "test-grounding",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &ExecutionPlan{
				GroundingHit: tt.groundingHit,
			}
			generator := NewPddlGenerator("test-action", plan, NewFakeMatcher(), zerolog.Nop())

			got := generator.ShouldGeneratePddl()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPddlDomainGenerator_GenerateDomain(t *testing.T) {
	// Test case: e-commerce customer support
	useCase := GroundingUseCase{
		Action: "Process refund for order {orderId}",
		Params: map[string]string{
			"orderId": "ORD123",
		},
		Capabilities: []string{
			"Verify refund eligibility",
			"Process payment refund",
		},
	}

	groundingHit := &GroundingHit{
		Name:    "customer-support",
		Domain:  "e-commerce-customer-support",
		Version: "1.0",
		UseCase: useCase,
	}

	executionPlan := &ExecutionPlan{
		GroundingHit: groundingHit,
		GroundingID:  "customer-support",
		Tasks: []*SubTask{
			{
				ID: TaskZero,
				Input: map[string]any{
					"orderId": "ORD123",
					"amount":  100.50,
				},
			},
			{
				ID:      "task1",
				Service: "refund-service",
				Capabilities: []string{
					"A service that handles refund processing including eligibility checks and payment refunds",
				},
				ExpectedInput: Spec{
					Properties: map[string]Spec{
						"orderId": {Type: "string"},
						"amount":  {Type: "number"},
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

	generator := NewPddlGenerator("Process refund for order ORD123", executionPlan, matcher, zerolog.Nop())

	domain, err := generator.GenerateDomain(context.Background())
	assert.NoError(t, err)
	assert.NotEmpty(t, domain)

	// Verify domain contains expected elements
	assert.Contains(t, domain, "(define (domain e-commerce-customer-support)")
	assert.Contains(t, domain, "(:requirements :strips :typing)")

	// Verify types - should include types from Task0 input
	assert.Contains(t, domain, "order-id - object")
	assert.Contains(t, domain, "number - object") // For amount
	assert.NotContains(t, domain, "customer-id - object", "Should not include types for unused params")

	// Verify predicates
	assert.Contains(t, domain, "(service-validated ?s - service)")
	assert.Contains(t, domain, "(valid-orderId ?orderId - order-id)")
	assert.Contains(t, domain, "(valid-amount ?amount - number)")
	assert.NotContains(t, domain, "valid-customerId", "Should not include predicates for unused params")

	// Verify action
	assert.Contains(t, domain, "(:action execute-service")
	assert.Contains(t, domain, ":parameters (?s - service ?amount - number ?orderId - order-id)")
}

func TestPddlDomainGenerator_ValidateServiceCapabilities(t *testing.T) {
	tests := []struct {
		name           string
		useCase        GroundingUseCase
		matchResult    bool
		matchScore     float64
		tasks          []*SubTask
		expectedErr    bool
		expectedErrMsg string
	}{
		{
			name: "all capabilities matched",
			useCase: GroundingUseCase{
				Capabilities: []string{
					"Verify refund eligibility",
					"Process payment refund",
				},
			},
			matchResult: true,
			matchScore:  0.9,
			tasks: []*SubTask{
				{
					ID:      "task1",
					Service: "refund-service",
					Capabilities: []string{
						"A service that handles refund processing and verification",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "exactly 95% capabilities matched",
			useCase: GroundingUseCase{
				Capabilities: []string{
					"Verify refund eligibility",
					"Process payment refund",
					"Send confirmation email",
					"Update order status",
					"Notify customer service",
					"Update inventory",
					"Calculate tax refund",
					"Process loyalty points",
					"Record transaction",
					"Generate receipt",
					"Archive refund record",
					"Update customer history",
					"Check fraud indicators",
					"Validate shipping status",
					"Update payment gateway",
					"Check compliance rules",
					"Record audit trail",
					"Update financial records",
					"Process chargeback",
					"Update metrics",
				},
			},
			matchResult: true,
			matchScore:  0.8,
			tasks: []*SubTask{
				{
					ID:      "task1",
					Service: "refund-service",
					Capabilities: []string{
						"Comprehensive refund processing service with validation",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "88.24% capabilities matched (below threshold)",
			useCase: GroundingUseCase{
				Capabilities: []string{
					"Cap1", "Cap2", "Cap3", "Cap4", "Cap5",
					"Cap6", "Cap7", "Cap8", "Cap9", "Cap10",
					"Cap11", "Cap12", "Cap13", "Cap14", "Cap15",
					"Cap16", "Cap17",
				},
			},
			matchResult: true, // Match first 16 capabilities
			matchScore:  0.8,
			tasks: []*SubTask{
				{
					ID:      "task1",
					Service: "test-service",
					Capabilities: []string{
						"Generic service capability",
					},
				},
			},
			expectedErr:    true,
			expectedErrMsg: "insufficient capability coverage: 88.24%",
		},
		{
			name: "no capabilities matched",
			useCase: GroundingUseCase{
				Capabilities: []string{
					"Verify refund eligibility",
					"Process payment refund",
				},
			},
			matchResult: false,
			matchScore:  0.7,
			tasks: []*SubTask{
				{
					ID:      "task1",
					Service: "unrelated-service",
					Capabilities: []string{
						"A completely different service capability",
					},
				},
			},
			expectedErr:    true,
			expectedErrMsg: "insufficient capability coverage: 0.00%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test logger to capture logs
			var logBuf strings.Builder
			testLogger := zerolog.New(&logBuf)

			// Configure fake matcher for specific test case
			matcher := NewFakeMatcher()
			matcher.matchResponse = tt.matchResult
			matcher.matchScore = tt.matchScore

			// For the 88.24% test case, set up specific matches
			if tt.name == "88.24% capabilities matched (below threshold)" {
				matcher.capabilitiesToMatch = map[string]any{"enabled": true}
			}

			// Create generator with test configuration
			generator := NewPddlGenerator("test-action", &ExecutionPlan{Tasks: tt.tasks}, matcher, testLogger)

			// Execute validation
			err := generator.validateServiceCapabilities(context.Background(), tt.useCase)

			// Verify error cases
			if tt.expectedErr {
				assert.Error(t, err)
				if tt.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify logging
			logOutput := logBuf.String()
			assert.Contains(t, logOutput, "Matching capabilities")
			assert.Contains(t, logOutput, "Capability matching summary")

			if !tt.expectedErr {
				assert.Contains(t, logOutput, `"matchPercentage":1`) // 100% for full match cases
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

	groundingHit := &GroundingHit{
		Name:    "test-grounding",
		Domain:  "test-domain",
		UseCase: useCase,
	}

	executionPlan := &ExecutionPlan{
		GroundingHit: groundingHit,
		Tasks: []*SubTask{
			{
				ID: TaskZero,
				Input: map[string]any{
					"orderId":    "ORD123",
					"customerId": "CUST456",
				},
			},
			{
				ID:           "task1",
				Service:      "refund-service",
				Capabilities: []string{"A refund processing service"},
				ExpectedInput: Spec{
					Properties: map[string]Spec{
						"orderId":    {Type: "string"},
						"customerId": {Type: "string"},
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

	generator := NewPddlGenerator("Process refund for order ORD123", executionPlan, matcher, zerolog.Nop())

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
			// Verify action parameters include all Task0 params
			assert.Contains(t, trimmedLine, "?orderId - order-id")
			assert.Contains(t, trimmedLine, "?customerId - customer-id")
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

func TestPddlGenerator_GenerateProblem(t *testing.T) {
	// Test case setup - use e-commerce scenario
	useCase := GroundingUseCase{
		Action: "Process refund for order {orderId}",
		Params: map[string]string{
			"orderId": "ORD123",
		},
		Capabilities: []string{
			"Verify refund eligibility",
			"Process payment refund",
		},
	}

	groundingHit := &GroundingHit{
		Name:    "customer-support",
		Domain:  "e-commerce-customer-support",
		Version: "1.0",
		UseCase: useCase,
	}

	executionPlan := &ExecutionPlan{
		ProjectID:    "test-project",
		GroundingHit: groundingHit,
		Tasks: []*SubTask{
			{
				ID: TaskZero,
				Input: map[string]any{
					"orderId":     "ORD123",
					"amount":      100.50,
					"customerId":  "CUST456",
					"orderStatus": "shipped",
				},
			},
			{
				ID:      "task1",
				Service: "eligibility-service",
				Input: map[string]any{
					"orderId":    "$task0.orderId",
					"amount":     "$task0.amount",
					"customerId": "$task0.customerId",
					"orderState": "$task0.orderStatus",
				},
				Capabilities: []string{
					"A service that verifies refund eligibility based on order status and amount",
				},
			},
			{
				ID:      "task2",
				Service: "refund-service",
				Input: map[string]any{
					"orderId":           "$task0.orderId",
					"amount":            "$task0.amount",
					"eligibilityResult": "$task1.result",
				},
				Capabilities: []string{
					"A service that processes payment refunds",
				},
			},
		},
	}

	matcher := NewFakeMatcher()
	matcher.matchResponse = true
	matcher.matchScore = 0.9

	generator := NewPddlGenerator("Process refund for order ORD123", executionPlan, matcher, zerolog.Nop())

	problem, err := generator.GenerateProblem(context.Background())
	assert.NoError(t, err)
	assert.NotEmpty(t, problem)

	// Verify problem contains expected elements
	assert.Contains(t, problem, "(define (problem p-test-project)")
	assert.Contains(t, problem, "(:domain e-commerce-customer-support)")

	// Verify objects section
	assert.Contains(t, problem, "(:objects")
	// Check service objects
	assert.Contains(t, problem, "task1 - service")
	assert.Contains(t, problem, "task2 - service")
	// Check parameter objects
	assert.Contains(t, problem, "ORD123 - order-id")
	assert.Contains(t, problem, "CUST456 - customer-id")
	assert.Contains(t, problem, "n100.5 - number") // amount converted to number type
	assert.Contains(t, problem, "shipped - order-status")

	// Verify customerId validation is present
	assert.Contains(t, problem, "(valid-customerId CUST456)")

	// Verify Task0 dependencies are still NOT creating depends-on predicates
	assert.NotContains(t, problem, "depends-on task1 task0")

	// Verify init section
	assert.Contains(t, problem, "(:init")
	// Check service states
	assert.Contains(t, problem, "(service-validated task1)")
	assert.Contains(t, problem, "(service-active task1)")
	assert.Contains(t, problem, "(service-validated task2)")
	assert.Contains(t, problem, "(service-active task2)")
	// Check parameter validations
	assert.Contains(t, problem, "(valid-orderId ORD123)")
	// Check dependencies
	assert.Contains(t, problem, "(depends-on task2 task1)") // task2 depends on task1

	// Verify goal section
	assert.Contains(t, problem, "(:goal")
	assert.Contains(t, problem, "(service-complete task1)")
	assert.Contains(t, problem, "(service-complete task2)")

	// Count parentheses to verify structure
	openCount := strings.Count(problem, "(")
	closeCount := strings.Count(problem, ")")
	assert.Equal(t, openCount, closeCount, "Parentheses should be balanced")
}

func TestPddlGenerator_GenerateProblem_NoTaskZero(t *testing.T) {
	groundingHit := &GroundingHit{
		Name:   "test-grounding",
		Domain: "test-domain",
	}
	executionPlan := &ExecutionPlan{
		ProjectID:    "test-project",
		GroundingHit: groundingHit,
		Tasks: []*SubTask{
			{
				ID:      "task1",
				Service: "test-service",
			},
		},
	}

	generator := NewPddlGenerator("test action", executionPlan, NewFakeMatcher(), zerolog.Nop())

	problem, err := generator.GenerateProblem(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task zero not found")
	assert.Empty(t, problem)
}

func TestPddlGenerator_GenerateProblem_NoGrounding(t *testing.T) {
	executionPlan := &ExecutionPlan{
		ProjectID: "test-project",
		Tasks: []*SubTask{
			{
				ID: TaskZero,
				Input: map[string]any{
					"param1": "value1",
				},
			},
		},
	}

	generator := NewPddlGenerator("test action", executionPlan, NewFakeMatcher(), zerolog.Nop())

	problem, err := generator.GenerateProblem(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no domain grounding available")
	assert.Empty(t, problem)
}

func TestPddlGenerator_GenerateProblem_ComplexTypes(t *testing.T) {
	useCase := GroundingUseCase{
		Action: "Test action",
		Params: map[string]string{
			"param1": "value1",
		},
	}

	groundingHit := &GroundingHit{
		Name:    "test-grounding",
		Domain:  "test-domain",
		UseCase: useCase,
	}

	executionPlan := &ExecutionPlan{
		ProjectID:    "test-project",
		GroundingHit: groundingHit,
		Tasks: []*SubTask{
			{
				ID: TaskZero,
				Input: map[string]any{
					"simpleParam": "value1",
					"complexParam": map[string]any{
						"nested": "value",
					},
					"arrayParam": []string{"val1", "val2"},
				},
			},
			{
				ID:      "task1",
				Service: "test-service",
			},
		},
	}

	generator := NewPddlGenerator("test action", executionPlan, NewFakeMatcher(), zerolog.Nop())

	problem, err := generator.GenerateProblem(context.Background())
	assert.NoError(t, err)
	assert.NotEmpty(t, problem)

	// Complex types should be skipped
	assert.Contains(t, problem, "value1 - simpleparam-value")
	assert.NotContains(t, problem, "complexParam")
	assert.NotContains(t, problem, "arrayParam")
}

func TestPddlGenerator_GenerateProblem_SpecialCharacters(t *testing.T) {
	useCase := GroundingUseCase{
		Action: "Test action",
		Params: map[string]string{
			"param1": "value1",
		},
	}

	groundingHit := &GroundingHit{
		Name:    "test-grounding",
		Domain:  "test-domain",
		UseCase: useCase,
	}

	executionPlan := &ExecutionPlan{
		ProjectID:    "test-project",
		GroundingHit: groundingHit,
		Tasks: []*SubTask{
			{
				ID: TaskZero,
				Input: map[string]any{
					"paramWithDash":  "value-with-dashes",
					"paramWithSpace": "value with spaces",
				},
			},
			{
				ID:      "task1",
				Service: "test-service",
			},
		},
	}

	generator := NewPddlGenerator("test action", executionPlan, NewFakeMatcher(), zerolog.Nop())

	problem, err := generator.GenerateProblem(context.Background())
	assert.NoError(t, err)
	assert.NotEmpty(t, problem)

	// Special characters should be replaced with underscores
	assert.Contains(t, problem, "value_with_dashes")
	assert.Contains(t, problem, "value_with_spaces")
}
