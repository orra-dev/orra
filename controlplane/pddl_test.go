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

func TestPDDLDomainGenerator_AddTypes(t *testing.T) {
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

			generator := &PDDLDomainGenerator{
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
