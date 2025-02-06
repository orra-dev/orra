/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestParseYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    *DomainExample
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid domain example",
			yaml: `
name: "customer-support-examples"
domain: "E-commerce Customer Support"
version: "1.0"
examples:
  - action: "Handle customer inquiry about {orderId}"
    params:
      orderId: "ORD123"
      query: "Where is my order?"
    capabilities:
      - "Lookup order details"
      - "Generate response"
    intent: "Customer wants order status and tracking"
constraints:
  - "Verify customer owns order"`,
			want: &DomainExample{
				Name:    "customer-support-examples",
				Domain:  "E-commerce Customer Support",
				Version: "1.0",
				Examples: []ActionExample{
					{
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
				},
				Constraints: []string{"Verify customer owns order"},
			},
			wantErr: false,
		},
		{
			name: "valid - multiple examples",
			yaml: `
name: "customer-support-examples"
domain: "E-commerce Customer Support"
version: "1.0"
examples:
  - action: "Handle customer inquiry about {orderId}"
    params:
      orderId: "ORD123"
    capabilities:
      - "Lookup order details"
    intent: "Check order status"
  - action: "List all orders"
    capabilities:
      - "Order listing"
    intent: "Show all orders"`,
			want: &DomainExample{
				Name:    "customer-support-examples",
				Domain:  "E-commerce Customer Support",
				Version: "1.0",
				Examples: []ActionExample{
					{
						Action: "Handle customer inquiry about {orderId}",
						Params: map[string]string{
							"orderId": "ORD123",
						},
						Capabilities: []string{"Lookup order details"},
						Intent:       "Check order status",
					},
					{
						Action:       "List all orders",
						Capabilities: []string{"Order listing"},
						Intent:       "Show all orders",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - missing required fields",
			yaml: `
name: "customer-support-examples"
domain: "E-commerce Customer Support"
examples:
  - action: "Handle customer inquiry about {orderId}"
    params:
      orderId: "ORD123"
    capabilities:
      - "Lookup order details"
    intent: "Check order status"`,
			wantErr: true,
			errMsg:  "version: cannot be blank",
		},
		{
			name: "invalid - malformed yaml",
			yaml: `
name: "customer-support-examples"
domain: "E-commerce Customer Support"
version: "1.0"
examples:
  - action: Handle customer inquiry
    capabilities:
      - Lookup order details
    intent: Check order status
    params: invalid-params-format`,
			wantErr: true,
			errMsg:  "yaml:",
		},
		{
			name: "invalid - example missing required field",
			yaml: `
name: "customer-support-examples"
domain: "E-commerce Customer Support"
version: "1.0"
examples:
  - action: "Handle customer inquiry about {orderId}"
    params:
      orderId: "ORD123"
    capabilities:
      - "Lookup order details"`,
			wantErr: true,
			errMsg:  "examples[0]: intent: cannot be blank",
		},
		{
			name: "invalid - params don't match variables",
			yaml: `
name: "customer-support-examples"
domain: "E-commerce Customer Support"
version: "1.0"
examples:
  - action: "Handle customer inquiry about {orderId}"
    params:
      customerId: "CUST123"
    capabilities:
      - "Lookup order details"
    intent: "Check order status"`,
			wantErr: true,
			errMsg:  "examples[0]: params: missing required param(s): [orderId]",
		},
		{
			name: "invalid - empty capabilities",
			yaml: `
name: "customer-support-examples"
domain: "E-commerce Customer Support"
version: "1.0"
examples:
  - action: "Handle customer inquiry about {orderId}"
    params:
      orderId: "ORD123"
    capabilities: []
    intent: "Check order status"`,
			wantErr: true,
			errMsg:  "examples[0]: capabilities: at least one capability is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DomainExample
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if tt.wantErr {
				if err == nil {
					err = got.Validate()
				}
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				err = got.Validate()
				assert.NoError(t, err)
				if tt.want != nil {
					assert.Equal(t, tt.want, &got)
				}
			}
		})
	}
}

func TestDomainExampleValidation(t *testing.T) {
	validActionWithVars := ActionExample{
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

	validActionNoVars := ActionExample{
		Action: "List all active orders",
		Capabilities: []string{
			"Order listing",
			"Status filtering",
		},
		Intent: "Show all current orders in the system",
	}

	tests := []struct {
		name    string
		example DomainExample
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid domain example with variable action",
			example: DomainExample{
				Name:        "customer-support-examples",
				Domain:      "E-commerce Customer Support",
				Version:     "1.0",
				Examples:    []ActionExample{validActionWithVars},
				Constraints: []string{"Verify customer owns order"},
			},
			wantErr: false,
		},
		{
			name: "valid domain example with non-variable action",
			example: DomainExample{
				Name:        "customer-support-examples",
				Domain:      "E-commerce Customer Support",
				Version:     "1.0",
				Examples:    []ActionExample{validActionNoVars},
				Constraints: []string{"Verify customer owns order"},
			},
			wantErr: false,
		},
		{
			name: "invalid name - too short",
			example: DomainExample{
				Name:     "cs",
				Domain:   "E-commerce Customer Support",
				Version:  "1.0",
				Examples: []ActionExample{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "name: must be between 3 and 63 characters",
		},
		{
			name: "invalid name - too long",
			example: DomainExample{
				Name:     "this-is-a-very-long-name-that-exceeds-the-maximum-length-limit-for-domain-examples",
				Domain:   "E-commerce Customer Support",
				Version:  "1.0",
				Examples: []ActionExample{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "name: must be between 3 and 63 characters",
		},
		{
			name: "invalid name - wrong characters",
			example: DomainExample{
				Name:     "Customer_Support",
				Domain:   "E-commerce Customer Support",
				Version:  "1.0",
				Examples: []ActionExample{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "name: must consist of lowercase alphanumeric characters",
		},
		{
			name: "invalid name - starts with dot",
			example: DomainExample{
				Name:     ".customer-support",
				Domain:   "E-commerce Customer Support",
				Version:  "1.0",
				Examples: []ActionExample{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "name: must consist of lowercase alphanumeric characters",
		},
		{
			name: "missing domain",
			example: DomainExample{
				Name:     "customer-support-examples",
				Version:  "1.0",
				Examples: []ActionExample{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "domain: cannot be blank",
		},
		{
			name: "missing version",
			example: DomainExample{
				Name:     "customer-support-examples",
				Domain:   "E-commerce Customer Support",
				Examples: []ActionExample{validActionWithVars},
			},
			wantErr: true,
			errMsg:  "version: cannot be blank",
		},
		{
			name: "empty examples",
			example: DomainExample{
				Name:     "customer-support-examples",
				Domain:   "E-commerce Customer Support",
				Version:  "1.0",
				Examples: []ActionExample{},
			},
			wantErr: true,
			errMsg:  "examples: cannot be empty",
		},
		{
			name: "invalid - empty constraint text",
			example: DomainExample{
				Name:        "customer-support-examples",
				Domain:      "E-commerce Customer Support",
				Version:     "1.0",
				Examples:    []ActionExample{validActionWithVars},
				Constraints: []string{"Valid constraint", "   ", "Another valid one"},
			},
			wantErr: true,
			errMsg:  "constraints: constraint at index 1 cannot be empty",
		},
		{
			name: "valid - no constraints",
			example: DomainExample{
				Name:     "customer-support-examples",
				Domain:   "E-commerce Customer Support",
				Version:  "1.0",
				Examples: []ActionExample{validActionWithVars},
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

func TestActionExampleValidation(t *testing.T) {
	tests := []struct {
		name    string
		example ActionExample
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid action example with variables",
			example: ActionExample{
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
			name: "valid action example without variables",
			example: ActionExample{
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
			example: ActionExample{
				Params:       map[string]string{"orderId": "ORD123"},
				Capabilities: []string{"Lookup order details"},
				Intent:       "Check order status",
			},
			wantErr: true,
			errMsg:  "action: cannot be blank",
		},
		{
			name: "missing params for action with variables",
			example: ActionExample{
				Action:       "Handle customer inquiry about {orderId}",
				Capabilities: []string{"Lookup order details"},
				Intent:       "Check order status",
			},
			wantErr: true,
			errMsg:  "params: params required when action contains variables: [orderId]",
		},
		{
			name: "missing param for variable",
			example: ActionExample{
				Action:       "Handle customer inquiry about {orderId}",
				Params:       map[string]string{"customerId": "CUST123"},
				Capabilities: []string{"Lookup order details"},
				Intent:       "Check order status",
			},
			wantErr: true,
			errMsg:  "params: missing required params: [orderId]",
		},
		{
			name: "empty capability",
			example: ActionExample{
				Action:       "List all active orders",
				Capabilities: []string{"", "Generate response"},
				Intent:       "Check order status",
			},
			wantErr: true,
			errMsg:  "capabilities: capability at index 0 is empty",
		},
		{
			name: "missing intent",
			example: ActionExample{
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
