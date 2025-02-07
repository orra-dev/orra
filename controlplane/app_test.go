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

// setupTestApp creates a new App instance with a test project for testing
func setupTestApp(t *testing.T) (*App, *Project) {
	t.Helper()

	app := &App{
		Router: mux.NewRouter(),
		Plane:  NewControlPlane(),
		Logger: zerolog.New(zerolog.NewTestWriter(t)),
	}

	app.Plane.Initialise(
		context.Background(),
		nil,
		nil,
		nil,
		app.Logger,
	)

	app.configureRoutes()

	// Create a test project
	project := &Project{
		ID:     app.Plane.GenerateProjectKey(),
		APIKey: app.Plane.GenerateAPIKey(),
	}
	app.Plane.projects[project.ID] = project

	return app, project
}

// createTestDomainExample returns a valid domain example for testing
func createTestDomainExample() *DomainExample {
	return &DomainExample{
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
		Constraints: []string{
			"Verify customer owns order",
		},
	}
}

func TestDomainExamplesAPI(t *testing.T) {
	app, project := setupTestApp(t)
	example := createTestDomainExample()

	t.Run("Add Domain Example Success", func(t *testing.T) {
		body, err := json.Marshal(example)
		if err != nil {
			t.Fatalf("Failed to marshal example: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/domain-examples", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", project.APIKey))

		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status code %d, got %d", http.StatusCreated, w.Code)
		}

		var response DomainExample
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		domainExamples := app.Plane.domainExamples[project.ID]
		if len(domainExamples) != 1 {
			t.Errorf("Expected the plane to have 1 domain example, got %d", len(domainExamples))
		}

		if response.Name != example.Name {
			t.Errorf("Expected name %s, got %s", example.Name, response.Name)
		}
	})

	t.Run("List Domain Examples Success", func(t *testing.T) {
		if err := app.Plane.AddDomainExample(project.ID, example); err != nil {
			t.Fatalf("Failed to add example: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/domain-examples", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", project.APIKey))

		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var examples []*DomainExample
		if err := json.NewDecoder(w.Body).Decode(&examples); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(examples) != 1 {
			t.Errorf("Expected 1 example, got %d", len(examples))
		}

		if examples[0].Name != example.Name {
			t.Errorf("Expected name %s, got %s", example.Name, examples[0].Name)
		}
	})

	t.Run("Remove Domain Example By Name Success", func(t *testing.T) {
		if err := app.Plane.AddDomainExample(project.ID, example); err != nil {
			t.Fatalf("Failed to add example: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/domain-examples/%s", example.Name), nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", project.APIKey))

		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("Expected status code %d, got %d", http.StatusNoContent, w.Code)
		}

		// Verify example was removed
		examples, err := app.Plane.GetDomainExamples(project.ID)
		if err != nil {
			t.Fatalf("Failed to get examples: %v", err)
		}
		if len(examples) != 0 {
			t.Errorf("Expected 0 examples after deletion, got %d", len(examples))
		}
	})

	t.Run("Remove All Domain Examples Success", func(t *testing.T) {
		// First add example back
		if err := app.Plane.AddDomainExample(project.ID, example); err != nil {
			t.Fatalf("Failed to add example: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, "/domain-examples", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", project.APIKey))

		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("Expected status code %d, got %d", http.StatusNoContent, w.Code)
		}

		examples, err := app.Plane.GetDomainExamples(project.ID)
		if err != nil {
			t.Fatalf("Failed to get examples: %v", err)
		}
		if len(examples) != 0 {
			t.Errorf("Expected 0 examples after deletion, got %d", len(examples))
		}
	})
}

func TestDomainExamplesAPIAuth(t *testing.T) {
	app, project := setupTestApp(t)
	example := createTestDomainExample()

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
			if err := app.Plane.AddDomainExample(project.ID, example); err != nil {
				t.Fatalf("Failed to add example: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/domain-examples", nil)
			if tt.apiKey != "" {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tt.apiKey))
			}

			w := httptest.NewRecorder()
			app.Router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status code %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestDomainExampleValidationViaApi(t *testing.T) {
	app, project := setupTestApp(t)

	tests := []struct {
		name       string
		example    *DomainExample
		wantStatus int
	}{
		{
			name: "Invalid Name Format",
			example: &DomainExample{
				Name:    "Invalid Name!",
				Domain:  "Test Domain",
				Version: "1.0",
				Examples: []ActionExample{
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
			name: "Missing Examples",
			example: &DomainExample{
				Name:     "valid-name",
				Domain:   "Test Domain",
				Version:  "1.0",
				Examples: []ActionExample{},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid Action Example",
			example: &DomainExample{
				Name:    "valid-name",
				Domain:  "Test Domain",
				Version: "1.0",
				Examples: []ActionExample{
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

			req := httptest.NewRequest(http.MethodPost, "/domain-examples", bytes.NewBuffer(body))
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
