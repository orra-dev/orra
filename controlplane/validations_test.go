/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"testing"

	v "github.com/RussellLuo/validating/v3"
	"github.com/stretchr/testify/assert"
)

func TestSpecValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    Spec
		wantErr bool
	}{
		{
			name: "valid object spec with properties",
			spec: Spec{
				Type: "object",
				Properties: map[string]Spec{
					"name": {
						Type: "string",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid type",
			spec: Spec{
				Type: "invalid",
				Properties: map[string]Spec{
					"name": {
						Type: "string",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing properties for object type",
			spec: Spec{
				Type: "object",
			},
			wantErr: true,
		},
		{
			name: "empty properties for object type",
			spec: Spec{
				Type:       "object",
				Properties: map[string]Spec{},
			},
			wantErr: true,
		},
		{
			name: "valid nested object spec",
			spec: Spec{
				Type: "object",
				Properties: map[string]Spec{
					"user": {
						Type: "object",
						Properties: map[string]Spec{
							"name": {
								Type: "string",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid primitive type in properties",
			spec: Spec{
				Type: "object",
				Properties: map[string]Spec{
					"age": {
						Type: "number",
					},
					"active": {
						Type: "boolean",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := v.Validate(tt.spec.Validation())
			if tt.wantErr {
				assert.NotEmpty(t, errs, "Expected validation errors")
			} else {
				assert.Empty(t, errs, "Expected no validation errors")
			}
		})
	}
}

func TestServiceSchemaValidation(t *testing.T) {
	tests := []struct {
		name    string
		schema  ServiceSchema
		wantErr bool
	}{
		{
			name: "valid service schema",
			schema: ServiceSchema{
				Input: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"name": {Type: "string"},
					},
				},
				Output: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"result": {Type: "string"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid input type - must be object",
			schema: ServiceSchema{
				Input: Spec{
					Type: "string",
				},
				Output: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"result": {Type: "string"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing properties in input object",
			schema: ServiceSchema{
				Input: Spec{Type: "object"},
				Output: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"result": {Type: "string"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := v.Validate(tt.schema.Validation())
			if tt.wantErr {
				assert.NotEmpty(t, errs, "Expected validation errors")
			} else {
				assert.Empty(t, errs, "Expected no validation errors")
			}
		})
	}
}

func TestServiceSchemaValidation_WithRevert(t *testing.T) {
	tests := []struct {
		name    string
		schema  ServiceSchema
		wantErr bool
	}{
		{
			name: "valid schema with revert",
			schema: ServiceSchema{
				Input: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"orderId": {Type: "string"},
					},
				},
				Output: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"status": {Type: "string"},
					},
				},
				Revert: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"transactionId": {Type: "string"},
						"amount":        {Type: "number"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid schema without revert",
			schema: ServiceSchema{
				Input: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"orderId": {Type: "string"},
					},
				},
				Output: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"status": {Type: "string"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid revert - not object type",
			schema: ServiceSchema{
				Input: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"orderId": {Type: "string"},
					},
				},
				Output: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"status": {Type: "string"},
					},
				},
				Revert: Spec{
					Type: "string",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := v.Validate(tt.schema.Validation())
			if tt.wantErr {
				assert.NotEmpty(t, errs, "Expected validation errors")
			} else {
				assert.Empty(t, errs, "Expected no validation errors")
			}
		})
	}
}

func TestServiceInfoValidation(t *testing.T) {
	tests := []struct {
		name    string
		info    ServiceInfo
		wantErr bool
	}{
		{
			name: "valid service info",
			info: ServiceInfo{
				Name:        "my-service",
				Description: "A test service",
				Schema: ServiceSchema{
					Input: Spec{
						Type: "object",
						Properties: map[string]Spec{
							"name": {Type: "string"},
						},
					},
					Output: Spec{
						Type: "object",
						Properties: map[string]Spec{
							"result": {Type: "string"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid name - too short",
			info: ServiceInfo{
				Name:        "ab",
				Description: "A test service",
				Schema: ServiceSchema{
					Input: Spec{
						Type: "object",
						Properties: map[string]Spec{
							"name": {Type: "string"},
						},
					},
					Output: Spec{
						Type: "object",
						Properties: map[string]Spec{
							"result": {Type: "string"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid name - wrong pattern",
			info: ServiceInfo{
				Name:        "MY-SERVICE",
				Description: "A test service",
				Schema: ServiceSchema{
					Input: Spec{
						Type: "object",
						Properties: map[string]Spec{
							"name": {Type: "string"},
						},
					},
					Output: Spec{
						Type: "object",
						Properties: map[string]Spec{
							"result": {Type: "string"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty description",
			info: ServiceInfo{
				Name:        "my-service",
				Description: "",
				Schema: ServiceSchema{
					Input: Spec{
						Type: "object",
						Properties: map[string]Spec{
							"name": {Type: "string"},
						},
					},
					Output: Spec{
						Type: "object",
						Properties: map[string]Spec{
							"result": {Type: "string"},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := v.Validate(tt.info.Validation())
			if tt.wantErr {
				assert.NotEmpty(t, errs, "Expected validation errors")
			} else {
				assert.Empty(t, errs, "Expected no validation errors")
			}
		})
	}
}
