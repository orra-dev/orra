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

func TestServiceSchema_Validation_Arrays(t *testing.T) {
	tests := []struct {
		name    string
		schema  ServiceSchema
		wantErr bool
	}{
		{
			name: "valid array property",
			schema: ServiceSchema{
				Input: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"tags": {
							Type: "array",
							Items: &Spec{
								Type: "string",
							},
						},
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
			name: "array without items spec",
			schema: ServiceSchema{
				Input: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"data": {
							Type: "array",
							// Missing Items spec
						},
					},
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
			name: "nested array property",
			schema: ServiceSchema{
				Input: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"matrix": {
							Type: "array",
							Items: &Spec{
								Type: "array",
								Items: &Spec{
									Type: "integer",
								},
							},
						},
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
			name: "array of objects",
			schema: ServiceSchema{
				Input: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"users": {
							Type: "array",
							Items: &Spec{
								Type: "object",
								Properties: map[string]Spec{
									"id":  {Type: "string"},
									"age": {Type: "integer"},
								},
							},
						},
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
			name: "invalid array items type",
			schema: ServiceSchema{
				Input: Spec{
					Type: "object",
					Properties: map[string]Spec{
						"data": {
							Type: "array",
							Items: &Spec{
								Type: "invalid_type",
							},
						},
					},
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
			name: "incorrect array as top level type",
			schema: ServiceSchema{
				Input: Spec{
					Type: "array",
					Items: &Spec{
						Type: "string",
					},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.schema.Validation()
			errs := v.Validate(schema)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("ServiceSchema.Validation() error = %v, wantErr %v", errs, tt.wantErr)
				for _, err := range errs {
					t.Logf("Validation error: %v", err)
				}
			}
		})
	}
}
