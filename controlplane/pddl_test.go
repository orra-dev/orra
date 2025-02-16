/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestPddlDomainGenerator_AddTypes(t *testing.T) {
	tests := []struct {
		name      string
		useCase   *GroundingUseCase
		plan      *ExecutionPlan
		expected  string
		expectErr bool
	}{
		{
			name: "basic_service_types",
			useCase: &GroundingUseCase{
				Action: "Process order {orderId}",
				Params: map[string]string{
					"orderId": "ORD123",
				},
			},
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID:      "task_1",
						Service: "order_service",
						ExpectedInput: Spec{
							Properties: map[string]Spec{
								"orderId": {
									Type: "string",
								},
							},
						},
					},
				},
			},
			expected: `  (:types
    id - object
    order-id - id
  )

`,
		},
		{
			name: "multiple_service_types",
			useCase: &GroundingUseCase{
				Action: "Ship order {orderId} to {location}",
				Params: map[string]string{
					"orderId":  "ORD123",
					"location": "STORE1",
				},
			},
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID:      "task_1",
						Service: "shipping_service",
						ExpectedInput: Spec{
							Properties: map[string]Spec{
								"orderId": {
									Type: "string",
								},
								"storeLocation": {
									Type: "string",
								},
							},
						},
					},
				},
			},
			expected: `  (:types
    id location - object
    order-id - id
    store-location - location
  )

`,
		},
		{
			name: "monetary_and_status_types",
			useCase: &GroundingUseCase{
				Action: "Process payment {amount} for order {orderId}",
				Params: map[string]string{
					"amount":  "99.99",
					"orderId": "ORD123",
				},
			},
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID:      "task_1",
						Service: "payment_service",
						ExpectedInput: Spec{
							Properties: map[string]Spec{
								"paymentAmount": {
									Type: "number",
								},
								"orderId": {
									Type: "string",
								},
								"orderStatus": {
									Type: "string",
								},
							},
						},
					},
				},
			},
			expected: `  (:types
    id number status - object
    monetary-value - number
    order-id - id
    order-status - status
  )

`,
		},
		{
			name: "skip_task_zero",
			useCase: &GroundingUseCase{
				Action: "Process order",
				Params: map[string]string{},
			},
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID:      "task_0",
						Service: "",
						Input: map[string]any{
							"raw_input": "some value",
						},
					},
					{
						ID:      "task_1",
						Service: "order_service",
						ExpectedInput: Spec{
							Properties: map[string]Spec{
								"orderId": {
									Type: "string",
								},
							},
						},
					},
				},
			},
			expected: `  (:types
    id - object
    order-id - id
  )

`,
		},
		{
			name: "mixed_property_types",
			useCase: &GroundingUseCase{
				Action: "Update inventory",
				Params: map[string]string{},
			},
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID:      "task_1",
						Service: "inventory_service",
						ExpectedInput: Spec{
							Properties: map[string]Spec{
								"productId": {
									Type: "string",
								},
								"inStock": {
									Type: "boolean",
								},
								"unitPrice": {
									Type: "number",
								},
								"items": {
									Type: "array",
								},
							},
						},
					},
				},
			},
			expected: `  (:types
    boolean collection id number - object
    monetary-value - number
    product-id - id
  )

`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var domain strings.Builder
			logger := zerolog.Nop()

			generator := &PddlDomainGenerator{
				executionPlan: tt.plan,
				logger:        logger,
			}

			generator.addTypes(&domain, tt.useCase)

			if tt.expectErr {
				// Add error checking when we add error cases
				return
			}

			assert.Equal(t, tt.expected, domain.String())
		})
	}
}

func TestPddlDomainGenerator_AddPredicates(t *testing.T) {
	tests := []struct {
		name     string
		useCase  *GroundingUseCase
		plan     *ExecutionPlan
		expected string
	}{
		{
			name: "order_processing_predicates",
			useCase: &GroundingUseCase{
				Action: "Process order {orderId}",
				Capabilities: []string{
					"Check order status",
					"Verify order location",
				},
				Params: map[string]string{
					"orderId": "ORD123",
				},
			},
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID:      "task_1",
						Service: "order_service",
						ExpectedInput: Spec{
							Properties: map[string]Spec{
								"orderId": {
									Type: "string",
								},
								"isRush": {
									Type: "boolean",
								},
							},
						},
						ExpectedOutput: Spec{
							Properties: map[string]Spec{
								"orderStatus": {
									Type: "string",
								},
							},
						},
					},
				},
			},
			expected: `  (:predicates
    (at-location ?obj - object ?location - location)
    (exists-order ?id - order-id)
    (has-status ?obj - order-id ?status - order-status)
    (is-rush ?obj - order)
  )

`,
		},
		{
			name: "payment_processing_predicates",
			useCase: &GroundingUseCase{
				Action: "Process payment for order",
				Capabilities: []string{
					"Check payment status",
				},
			},
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID:      "task_1",
						Service: "payment_service",
						ExpectedInput: Spec{
							Properties: map[string]Spec{
								"paymentId": {
									Type: "string",
								},
								"isRefund": {
									Type: "boolean",
								},
							},
						},
					},
				},
			},
			expected: `  (:predicates
    (exists-payment ?id - payment-id)
    (has-status ?obj - payment-id ?status - payment-status)
    (is-refund ?obj - payment)
  )

`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var domain strings.Builder
			logger := zerolog.Nop()

			generator := &PddlDomainGenerator{
				executionPlan: tt.plan,
				logger:        logger,
			}

			generator.addPredicates(&domain, tt.useCase)
			assert.Equal(t, tt.expected, domain.String())
		})
	}
}

func TestPddlDomainGenerator_AddActions(t *testing.T) {
	tests := []struct {
		name     string
		useCase  *GroundingUseCase
		plan     *ExecutionPlan
		expected string
	}{
		{
			name: "order_processing_action",
			useCase: &GroundingUseCase{
				Action: "Process order {orderId}",
				Capabilities: []string{
					"Check order status",
					"Verify order location",
				},
				Params: map[string]string{
					"orderId": "ORD123",
				},
			},
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID:      "task_1",
						Service: "order_service",
						ExpectedInput: Spec{
							Properties: map[string]Spec{
								"orderId": {
									Type: "string",
								},
							},
						},
						ExpectedOutput: Spec{
							Properties: map[string]Spec{
								"orderStatus": {
									Type: "string",
								},
							},
						},
					},
				},
			},
			expected: `  (:action process-order
   :parameters (?order-id - order-id ?new-status - order-status ?loc - location)
   :precondition (and
    (exists-order ?order-id)
   )
   :effect (and
    (has-status ?order-id ?new-status)
    (at-location ?order-id ?loc)
   )
  )

`,
		},
		{
			name: "payment_processing_action",
			useCase: &GroundingUseCase{
				Action: "Process payment",
				Capabilities: []string{
					"Check payment status",
				},
			},
			plan: &ExecutionPlan{
				Tasks: []*SubTask{
					{
						ID:      "task_1",
						Service: "payment_service",
						ExpectedInput: Spec{
							Properties: map[string]Spec{
								"paymentId": {
									Type: "string",
								},
								"amount": {
									Type: "number",
								},
							},
						},
						ExpectedOutput: Spec{
							Properties: map[string]Spec{
								"paymentStatus": {
									Type: "string",
								},
							},
						},
					},
				},
			},
			expected: `  (:action process-payment
   :parameters (?payment-id - payment-id ?amount - monetary-value ?new-status - payment-status)
   :precondition (and
    (exists-payment ?payment-id)
   )
   :effect (and
    (has-status ?payment-id ?new-status)
   )
  )

`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var domain strings.Builder
			logger := zerolog.Nop()

			generator := &PddlDomainGenerator{
				executionPlan: tt.plan,
				logger:        logger,
			}

			generator.addActions(&domain, tt.useCase)
			assert.Equal(t, tt.expected, domain.String())
		})
	}
}

func TestPddlDomainGenerator_InferDetailedType(t *testing.T) {
	tests := []struct {
		name     string
		jsonType string
		propName string
		expected string
	}{
		{
			name:     "camel_case_id",
			jsonType: "string",
			propName: "orderId",
			expected: "order-id",
		},
		{
			name:     "underscore_id",
			jsonType: "string",
			propName: "order_id",
			expected: "order-id",
		},
		{
			name:     "uppercase_id",
			jsonType: "string",
			propName: "OrderID",
			expected: "order-id",
		},
		{
			name:     "simple_id",
			jsonType: "string",
			propName: "id",
			expected: "id",
		},
		// Add more cases for other types...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := &PddlDomainGenerator{
				logger: zerolog.Nop(),
			}

			actual := generator.inferDetailedType(tt.jsonType, tt.propName)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestPddlDomainGenerator_NormalizeParamName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple_camel_case_id",
			input:    "orderId",
			expected: "order-id",
		},
		{
			name:     "underscore_id",
			input:    "order_id",
			expected: "order-id",
		},
		{
			name:     "pascal_case_id",
			input:    "OrderId",
			expected: "order-id",
		},
		{
			name:     "uppercase_id",
			input:    "OrderID",
			expected: "order-id",
		},
		{
			name:     "camel_case_non_id",
			input:    "storeLocation",
			expected: "store-location",
		},
		{
			name:     "pascal_case_non_id",
			input:    "StoreLocation",
			expected: "store-location",
		},
		{
			name:     "with_acronym",
			input:    "userAPIKey",
			expected: "user-api-key",
		},
		{
			name:     "simple_word",
			input:    "name",
			expected: "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := &PddlDomainGenerator{
				logger: zerolog.Nop(),
			}

			actual := generator.normalizeParamName(tt.input)
			assert.Equal(t, tt.expected, actual,
				"Input: %s, Expected: %s, Got: %s",
				tt.input, tt.expected, actual)
		})
	}
}
