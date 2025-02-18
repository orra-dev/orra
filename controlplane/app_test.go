/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
)

type fakePddlValidator struct{}

func (f *fakePddlValidator) Validate(_ context.Context, _, _, _ string) error {
	return nil
}
func (f *fakePddlValidator) HealthCheck(_ context.Context) error { return nil }

// setupTestApp creates a new App instance with a test project for testing
func setupTestApp(t *testing.T) (*App, *Project) {
	t.Helper()

	app := &App{
		Router: mux.NewRouter(),
		Plane:  NewControlPlane(),
		Logger: zerolog.New(zerolog.NewTestWriter(t)),
	}

	app.Plane.Initialise(context.Background(), nil, nil, nil, &fakePddlValidator{}, nil, app.Logger)

	app.configureRoutes()

	// Create a test project
	project := &Project{
		ID:     app.Plane.GenerateProjectKey(),
		APIKey: app.Plane.GenerateAPIKey(),
	}
	app.Plane.projects[project.ID] = project

	return app, project
}

// createTestGroundingSpec returns a valid domain grounding spec for testing
func createTestGroundingSpec() *GroundingSpec {
	return &GroundingSpec{
		Name:    "customer-support-examples",
		Domain:  "e-commerce-customer-support",
		Version: "1.0",
		UseCases: []GroundingUseCase{
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
		Constraints: []string{
			"Verify customer owns order",
		},
	}
}

func TestGroundingAPI(t *testing.T) {
	app, project := setupTestApp(t)
	expected := createTestGroundingSpec()

	t.Run("Add Domain Grounding Spec Success", func(t *testing.T) {
		body, err := json.Marshal(expected)
		if err != nil {
			t.Fatalf("Failed to marshal expected: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/groundings", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", project.APIKey))

		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status code %d, got %d", http.StatusCreated, w.Code)
		}

		var actual GroundingSpec
		if err := json.NewDecoder(w.Body).Decode(&actual); err != nil {
			t.Fatalf("Failed to decode actual: %v", err)
		}

		groundings := app.Plane.groundings[project.ID]
		if len(groundings) != 1 {
			t.Errorf("Expected the plane to have 1 domain expected, got %d", len(groundings))
		}

		if actual.Name != expected.Name {
			t.Errorf("Expected name %s, got %s", expected.Name, actual.Name)
		}
	})

	t.Run("List Domain Grounding Specs Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/groundings", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", project.APIKey))

		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var specs []*GroundingSpec
		if err := json.NewDecoder(w.Body).Decode(&specs); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(specs) != 1 {
			t.Errorf("Expected 1 expected, got %d", len(specs))
		}

		if specs[0].Name != expected.Name {
			t.Errorf("Expected name %s, got %s", expected.Name, specs[0].Name)
		}
	})

	t.Run("Remove Domain Grounding Spec By Name Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/groundings/%s", expected.Name), nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", project.APIKey))

		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("Expected status code %d, got %d", http.StatusNoContent, w.Code)
		}

		// Verify expected was removed
		specs := app.Plane.GetGroundingSpecs(project.ID)
		if len(specs) != 0 {
			t.Errorf("Expected 0 specs after deletion, got %d", len(specs))
		}
	})

	t.Run("Remove All Domain Grounding For A Project Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/groundings", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", project.APIKey))

		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("Expected status code %d, got %d", http.StatusNoContent, w.Code)
		}

		examples := app.Plane.GetGroundingSpecs(project.ID)
		if len(examples) != 0 {
			t.Errorf("Expected 0 examples after deletion, got %d", len(examples))
		}
	})
}

func TestGroundingAPIAuth(t *testing.T) {
	app, project := setupTestApp(t)
	example := createTestGroundingSpec()

	tests := []struct {
		name       string
		apiKey     string
		wantStatus int
	}{
		{
			name:       "Invalid API Key",
			apiKey:     "invalid-key",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing API Key",
			apiKey:     "",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := app.Plane.ApplyGroundingSpec(project.ID, example); err != nil {
				t.Fatalf("Failed to add example: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/groundings", nil)
			if tt.apiKey != "" {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tt.apiKey))
			}

			w := httptest.NewRecorder()
			app.Router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status code %d, got %d", tt.wantStatus, w.Code)
			}

			if err := app.Plane.RemoveProjectGrounding(project.ID); err != nil {
				t.Fatalf("Failed to remove groundings: %v", err)
			}
		})
	}
}

func TestGroundingValidationViaApi(t *testing.T) {
	app, project := setupTestApp(t)

	tests := []struct {
		name       string
		example    *GroundingSpec
		wantStatus int
	}{
		{
			name: "Invalid Name Format",
			example: &GroundingSpec{
				Name:    "Invalid Name!",
				Domain:  "Test Domain",
				Version: "1.0",
				UseCases: []GroundingUseCase{
					{
						Action:       "Test action",
						Capabilities: []string{"test"},
						Intent:       "test",
					},
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Missing UseCases",
			example: &GroundingSpec{
				Name:     "valid-name",
				Domain:   "Test Domain",
				Version:  "1.0",
				UseCases: []GroundingUseCase{},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid Action Example",
			example: &GroundingSpec{
				Name:    "valid-name",
				Domain:  "Test Domain",
				Version: "1.0",
				UseCases: []GroundingUseCase{
					{
						Action:       "",
						Capabilities: []string{"test"},
						Intent:       "test",
					},
				},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.example)
			if err != nil {
				t.Fatalf("Failed to marshal example: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/groundings", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", project.APIKey))

			w := httptest.NewRecorder()
			app.Router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status code %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}
