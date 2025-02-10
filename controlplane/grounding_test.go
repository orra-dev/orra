/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroundingSpecValidation(t *testing.T) {
	validActionWithVars := GroundingUseCase{
		Action: "Handle customer inquiry about {orderId}",
		Params: map[string]string{
			"orderId": "ORD123",
			"query":   "Where is my order?",
		},
		Capabilities: []string{
			"Lookup order details",
			"Generate response",
		},
		Intent: "Customer wants order status and tracking",
	}

	validActionNoVars := GroundingUseCase{
		Action: "List all active orders",
		Capabilities: []string{
			"Order listing",
			"Status filtering",
		},
		Intent: "Show all current orders in the system",
	}

	tests := []struct {
		name    string
		example GroundingSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid domain example with variable action",
			example: GroundingSpec{
				Name:        "customer-support-examples",
				Domain:      "e-commerce-customer-support",
				Version:     "1.0",
				UseCases:    []GroundingUseCase{validActionWithVars},
				Constraints: []string{"Verify customer owns order"},
			},
			wantErr: false,
		},
		{
			name: "valid domain example with non-variable action",
			example: GroundingSpec{
				Name:        "customer-support-examples",
				Domain:      "e-commerce-customer-support",
				Version:     "1.0",
				UseCases:    []GroundingUseCase{validActionNoVars},
				Constraints: []string{"Verify customer owns order"},
			},
			wantErr: false,
		},
		{
			name: "invalid name - too short",
			example: GroundingSpec{
				Name:     "cs",
				Domain:   "e-commerce-customer-support",
				Version:  "1.0",
				UseCases: []GroundingUseCase{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "name: must be between 3 and 63 characters",
		},
		{
			name: "invalid name - too long",
			example: GroundingSpec{
				Name:     "this-is-a-very-long-name-that-exceeds-the-maximum-length-limit-for-domain-examples",
				Domain:   "e-commerce-customer-support",
				Version:  "1.0",
				UseCases: []GroundingUseCase{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "name: must be between 3 and 63 characters",
		},
		{
			name: "invalid name - wrong characters",
			example: GroundingSpec{
				Name:     "Customer_Support",
				Domain:   "e-commerce-customer-support",
				Version:  "1.0",
				UseCases: []GroundingUseCase{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "name: must consist of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "invalid name - starts with dot",
			example: GroundingSpec{
				Name:     ".customer-support",
				Domain:   "e-commerce-customer-support",
				Version:  "1.0",
				UseCases: []GroundingUseCase{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "name: must consist of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "missing domain",
			example: GroundingSpec{
				Name:     "customer-support-examples",
				Version:  "1.0",
				UseCases: []GroundingUseCase{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "domain: must be between 3 and 63 characters",
		},
		{
			name: "missing version",
			example: GroundingSpec{
				Name:     "customer-support-examples",
				Domain:   "e-commerce-customer-support",
				UseCases: []GroundingUseCase{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "version: cannot be blank",
		},
		{
			name: "empty examples",
			example: GroundingSpec{
				Name:     "customer-support-examples",
				Domain:   "e-commerce-customer-support",
				Version:  "1.0",
				UseCases: []GroundingUseCase{},
			},
			wantErr: true,
			errMsg:  "use-cases: cannot be empty",
		},
		{
			name: "invalid - empty constraint text",
			example: GroundingSpec{
				Name:        "customer-support-examples",
				Domain:      "e-commerce-customer-support",
				Version:     "1.0",
				UseCases:    []GroundingUseCase{validActionWithVars},
				Constraints: []string{"Valid constraint", "   ", "Another valid one"},
			},
			wantErr: true,
			errMsg:  "constraints: constraint at index 1 cannot be empty",
		},
		{
			name: "valid - no constraints",
			example: GroundingSpec{
				Name:     "customer-support-examples",
				Domain:   "e-commerce-customer-support",
				Version:  "1.0",
				UseCases: []GroundingUseCase{validActionWithVars},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.example.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGroundingUseCasesValidation(t *testing.T) {
	tests := []struct {
		name    string
		example GroundingUseCase
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid action with variables",
			example: GroundingUseCase{
				Action: "Handle customer inquiry about {orderId}",
				Params: map[string]string{
					"orderId": "ORD123",
					"query":   "Where is my order?",
				},
				Capabilities: []string{
					"Lookup order details",
					"Generate response",
				},
				Intent: "Customer wants order status and tracking",
			},
			wantErr: false,
		},
		{
			name: "valid action without variables",
			example: GroundingUseCase{
				Action: "List all active orders",
				Capabilities: []string{
					"Order listing",
					"Status filtering",
				},
				Intent: "Show all current orders in the system",
			},
			wantErr: false,
		},
		{
			name: "missing action",
			example: GroundingUseCase{
				Params:       map[string]string{"orderId": "ORD123"},
				Capabilities: []string{"Lookup order details"},
				Intent:       "Check order status",
			},
			wantErr: true,
			errMsg:  "action: cannot be blank",
		},
		{
			name: "missing params for action with variables",
			example: GroundingUseCase{
				Action:       "Handle customer inquiry about {orderId}",
				Capabilities: []string{"Lookup order details"},
				Intent:       "Check order status",
			},
			wantErr: true,
			errMsg:  "params: params required when action contains variables: [orderId]",
		},
		{
			name: "missing param for variable",
			example: GroundingUseCase{
				Action:       "Handle customer inquiry about {orderId}",
				Params:       map[string]string{"customerId": "CUST123"},
				Capabilities: []string{"Lookup order details"},
				Intent:       "Check order status",
			},
			wantErr: true,
			errMsg:  "params: missing required param(s): [orderId]",
		},
		{
			name: "empty capability",
			example: GroundingUseCase{
				Action:       "List all active orders",
				Capabilities: []string{"", "Generate response"},
				Intent:       "Check order status",
			},
			wantErr: true,
			errMsg:  "capabilities: capability at index 0 is empty",
		},
		{
			name: "missing intent",
			example: GroundingUseCase{
				Action:       "List all active orders",
				Capabilities: []string{"Lookup order details"},
			},
			wantErr: true,
			errMsg:  "intent: cannot be blank",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.example.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
