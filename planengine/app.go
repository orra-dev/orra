/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gilcrest/diygoapi/errs"
	"github.com/gorilla/mux"
	"github.com/olahol/melody"
	"github.com/rs/zerolog"
)

type App struct {
	Engine       *PlanEngine
	Router       *mux.Router
	Db           *BadgerDB
	Cfg          Config
	TelemetrySvc *TelemetryService
	RootCtx      context.Context
	RootCancel   context.CancelFunc
	Logger       zerolog.Logger
}

func NewApp(cfg Config, args []string) (*App, error) {
	lgr, err := NewLogger(args)
	if err != nil {
		return nil, err
	}

	return &App{
		Logger: lgr,
		Cfg:    cfg,
	}, nil
}

func (app *App) configureRoutes() *App {
	app.Router.Use(app.VersionHeaderMiddleware)

	app.Router.HandleFunc("/health", app.healthHandler).Methods(http.MethodGet)
	app.Router.HandleFunc("/register/project", app.RegisterProject).Methods(http.MethodPost)
	app.Router.HandleFunc("/apikeys", app.APIKeyMiddleware(app.CreateAdditionalApiKey)).Methods(http.MethodPost)
	app.Router.HandleFunc("/webhooks", app.APIKeyMiddleware(app.AddWebhook)).Methods(http.MethodPost)
	app.Router.HandleFunc("/compensation-failure-webhooks", app.APIKeyMiddleware(app.AddCompensationFailureWebhook)).Methods(http.MethodPost)
	app.Router.HandleFunc("/compensation-failures", app.APIKeyMiddleware(app.ListCompensationFailuresHandler)).Methods(http.MethodGet)
	app.Router.HandleFunc("/compensation-failures/{id}", app.APIKeyMiddleware(app.GetCompensationFailureHandler)).Methods(http.MethodGet)
	app.Router.HandleFunc("/compensation-failures/{id}/resolve", app.APIKeyMiddleware(app.ResolveCompensationFailureHandler)).Methods(http.MethodPost)
	app.Router.HandleFunc("/compensation-failures/{id}/ignore", app.APIKeyMiddleware(app.IgnoreCompensationFailureHandler)).Methods(http.MethodPost)
	app.Router.HandleFunc("/register/service", app.APIKeyMiddleware(app.RegisterService)).Methods(http.MethodPost)
	app.Router.HandleFunc("/orchestrations", app.APIKeyMiddleware(app.OrchestrationsHandler)).Methods(http.MethodPost)
	app.Router.HandleFunc("/orchestrations", app.APIKeyMiddleware(app.ListOrchestrationsHandler)).Methods(http.MethodGet)
	app.Router.HandleFunc("/orchestrations/inspections/{id}", app.APIKeyMiddleware(app.OrchestrationInspectionHandler)).Methods(http.MethodGet)
	app.Router.HandleFunc("/register/agent", app.APIKeyMiddleware(app.RegisterAgent)).Methods(http.MethodPost)
	app.Router.HandleFunc("/ws", app.HandleWebSocket)
	app.Router.HandleFunc("/groundings", app.APIKeyMiddleware(app.ApplyGrounding)).Methods(http.MethodPost)
	app.Router.HandleFunc("/groundings", app.APIKeyMiddleware(app.ListGrounding)).Methods(http.MethodGet)
	app.Router.HandleFunc("/groundings/{name}", app.APIKeyMiddleware(app.RemoveGrounding)).Methods(http.MethodDelete)
	app.Router.HandleFunc("/groundings", app.APIKeyMiddleware(app.RemoveAllGrounding)).Methods(http.MethodDelete)

	return app
}

func (app *App) configureWebSocket() {
	app.Engine.WebSocketManager.melody.HandleConnect(func(s *melody.Session) {
		apiKey := s.Request.URL.Query().Get("apiKey")
		project, err := app.Engine.GetProjectByApiKey(apiKey)
		if err != nil {
			app.Logger.Error().Err(err).Msg("Invalid API key for WebSocket connection")
			return
		}
		svcID := s.Request.URL.Query().Get("serviceId")
		svcName, err := app.Engine.GetServiceName(project.ID, svcID)
		if err != nil {
			app.Logger.Error().Err(err).Msg("Unknown service for WebSocket connection")
			return
		}
		app.Engine.WebSocketManager.HandleConnection(svcID, svcName, s)
	})

	app.Engine.WebSocketManager.melody.HandleDisconnect(func(s *melody.Session) {
		serviceID, exists := s.Get("serviceID")
		if !exists {
			app.Logger.Error().Msg("serviceID missing from disconnected session")
			return
		}
		app.Engine.WebSocketManager.HandleDisconnection(serviceID.(string))
	})

	app.Engine.WebSocketManager.melody.HandleMessage(func(s *melody.Session, msg []byte) {
		app.Engine.WebSocketManager.HandleMessage(s, msg, func(serviceID string) (*ServiceInfo, error) {
			return app.Engine.GetServiceByID(serviceID)
		})
	})
}

func (app *App) Run() {
	port := app.Cfg.Port
	addr := fmt.Sprintf(":%d", port)

	srv := &http.Server{
		Addr: addr,
		// Good practice to set timeouts to avoid Slowloris attacks.
		// See: https://en.wikipediapp.org/wiki/Slowloris_(computer_security)
		WriteTimeout: time.Second * 180,
		ReadTimeout:  time.Second * 180,
		IdleTimeout:  time.Second * 180,
		Handler:      app.Router,
	}

	// Set up our server in s goroutine so that it doesn't block.
	go func() {
		app.Logger.Info().Msgf("Starting plan engine on %s", addr)
		if err := srv.ListenAndServe(); err != nil {
			app.Logger.Info().Msg(err.Error())
		}

		app.TelemetrySvc.TrackEvent(EventServerStart, map[string]any{
			"version": Version,
		})
	}()

	c := make(chan os.Signal, 1)

	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Block until we receive our signal.
	<-c

	// Create s deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.

	app.gracefulShutdown(srv, ctx)
}

func (app *App) gracefulShutdown(srv *http.Server, ctx context.Context) {
	app.RootCancel()

	if err := app.Engine.CancelAnyActiveOrchestrations(); err != nil {
		app.Logger.Error().Err(err).Msg("")
	}
	app.Logger.Info().Msg("Plan Engine shutting down")

	if err := app.Db.Close(); err != nil {
		app.Logger.Error().Err(err).Msg("DB shutdown error")
	}
	app.Logger.Info().Msg("DB shutdown complete")

	if err := srv.Shutdown(ctx); err != nil {
		app.Logger.Error().Err(err).Msg("Error shutting down plan engine server")
	}
	app.Logger.Debug().Msg("http: All connections drained")

	app.TelemetrySvc.TrackEvent(EventServerStop, map[string]any{
		"version": Version,
	})
}

func (app *App) RegisterProject(w http.ResponseWriter, r *http.Request) {
	var project Project
	if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}

	project.ID = app.Engine.GenerateProjectKey()
	project.APIKey = app.Engine.GenerateAPIKey()

	if err := app.Engine.AddProject(&project); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, errs.Code(ProjectRegistrationFailedErrCode), err))
		return
	}

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(project); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}
}

func (app *App) RegisterServiceOrAgent(w http.ResponseWriter, r *http.Request, serviceType ServiceType) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	var service ServiceInfo
	if err := json.NewDecoder(r.Body).Decode(&service); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}

	service.ProjectID = project.ID
	service.Type = serviceType

	if err := app.Engine.RegisterOrUpdateService(&service); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":         service.ID,
		"name":       service.Name,
		"status":     Registered,
		"revertible": service.Revertible,
		"version":    service.Version,
	}); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}
}

func (app *App) RegisterService(w http.ResponseWriter, r *http.Request) {
	app.RegisterServiceOrAgent(w, r, Service)
}

func (app *App) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	app.RegisterServiceOrAgent(w, r, Agent)
}

func (app *App) OrchestrationsHandler(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	var orchestration Orchestration
	if err := json.NewDecoder(r.Body).Decode(&orchestration); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}

	if err := app.Engine.PrepareOrchestration(app.RootCtx, project.ID, &orchestration, app.Engine.GetGroundingSpecs(project.ID)); err != nil {
		app.Logger.
			Error().
			Err(err).
			Str("AttemptedOrchestration", orchestration.ID).
			Str("Status", orchestration.Status.String()).
			Str("Action", orchestration.Action.Content).
			Msgf("Action cannot be executed")

		if orchestration.Status == NotActionable {
			errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Code(ActionNotActionableErrCode), err))
		} else {
			errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, errs.Code(ActionCannotExecuteErrCode), err))
		}
		return
	}

	app.Logger.Debug().Msgf("About to execute orchestration %s", orchestration.ID)
	go app.Engine.ExecuteOrchestration(app.RootCtx, &orchestration)
	w.WriteHeader(http.StatusAccepted)

	data, err := json.Marshal(orchestration)
	if err != nil {
		app.Logger.Error().Err(err).Interface("orchestration", orchestration).Msg("")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(data); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}
}

func (app *App) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	serviceID := r.URL.Query().Get("serviceId")

	// Perform API key authentication
	apiKey := r.URL.Query().Get("apiKey")
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		app.Logger.Error().Err(err).Msg("Invalid API key for WebSocket connection")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unauthorized, err))
		return
	}

	if !app.Engine.ServiceBelongsToProject(serviceID, project.ID) {
		app.Logger.Error().Str("serviceID", serviceID).Msg("Service not found for the given project")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unauthorized, err))
		return
	}

	if err := app.Engine.WebSocketManager.melody.HandleRequest(w, r); err != nil {
		app.Logger.Error().Str("serviceID", serviceID).Msg("Failed to handle request using the WebSocket")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}
}

func (app *App) CreateAdditionalApiKey(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	newApiKey := app.Engine.GenerateAPIKey()
	if err := app.Engine.AddProjectAPIKey(project.ID, newApiKey); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, errs.Code(ProjectAPIKeyAdditionFailedErrCode), err))
		return
	}

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"apiKey": newApiKey,
	}); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}
}

func (app *App) AddWebhook(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	var webhook struct {
		Url string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}

	if _, err := url.ParseRequestURI(webhook.Url); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Validation, err))
		return
	}

	if err := app.Engine.AddProjectWebhook(project.ID, webhook.Url); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Code(ProjectWebhookAdditionFailedErrCode), err))
		return
	}

	project.Webhooks = append(project.Webhooks, webhook.Url)

	// Return the new key
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(webhook); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}
}

func (app *App) AddCompensationFailureWebhook(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	var webhook struct {
		Url string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}

	if _, err := url.ParseRequestURI(webhook.Url); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Validation, err))
		return
	}

	if err := app.Engine.AddProjectCompensationFailureWebhook(project.ID, webhook.Url); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Code(ProjectWebhookAdditionFailedErrCode), err))
		return
	}

	// Return the webhook information
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(webhook); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}
}

func (app *App) ListOrchestrationsHandler(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	orchestrationList := app.Engine.GetOrchestrationList(project.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(orchestrationList); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Internal, err))
		return
	}
}

func (app *App) OrchestrationInspectionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	orchestrationID := vars["id"]

	if !app.Engine.OrchestrationBelongsToProject(orchestrationID, project.ID) {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.NotExist, errs.Code(UnknownOrchestrationErrCode), "unknown orchestration: "+orchestrationID))
		return
	}

	inspection, err := app.Engine.InspectOrchestration(orchestrationID)
	if err != nil {
		app.Logger.
			Error().
			Err(err).
			Str("OrchestrationID", orchestrationID).
			Msg("Failed to inspect orchestration")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(inspection); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Internal, err))
		return
	}
}

func (app *App) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]any{}); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Internal, err))
		return
	}
}

// ApplyGrounding apply new domain grounding spec to a project
func (app *App) ApplyGrounding(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	app.Logger.Info().Interface("project", project).Msg("ApplyGrounding")

	var grounding GroundingSpec
	if err := json.NewDecoder(r.Body).Decode(&grounding); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}

	grounding.ProjectID = project.ID

	if err := app.Engine.ApplyGroundingSpec(app.RootCtx, &grounding); err != nil {
		var validErr ValidationError
		if errors.As(err, &validErr) {
			errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, errs.Parameter(validErr.Field()), validErr.Error()))
			return
		}

		var specErr SpecVersionError
		if errors.As(err, &specErr) {
			errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Invalid, errs.Parameter("version"), specErr.Error()))
			return
		}

		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	w.WriteHeader(http.StatusCreated)
	app.Logger.Trace().Interface("Grounding", grounding).Msg("Successfully applied grounding spec")

	if err := json.NewEncoder(w).Encode(grounding); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}
}

// ListGrounding retrieves all domain grounding for a project
func (app *App) ListGrounding(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	groundings := app.Engine.GetGroundingSpecs(project.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(groundings); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Internal, errs.Code(JSONMarshalingFailErrCode), err))
		return
	}
}

// RemoveGrounding removes a specific domain grounding spec from a project
func (app *App) RemoveGrounding(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	if err := app.Engine.RemoveGroundingSpecByName(project.ID, name); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveAllGrounding removes domain grounding for a specific project
func (app *App) RemoveAllGrounding(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	if err := app.Engine.RemoveProjectGrounding(project.ID); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListCompensationFailuresHandler retrieves all failed compensations for a project
func (app *App) ListCompensationFailuresHandler(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	compensations, err := app.Engine.ListProjectFailedCompensations(project.ID)
	if err != nil {
		app.Logger.Error().Err(err).Str("projectID", project.ID).Msg("Failed to list failed compensations")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(compensations); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Internal, err))
		return
	}
}

// GetCompensationFailureHandler retrieves a specific failed compensation by ID
func (app *App) GetCompensationFailureHandler(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, "compensation failure ID is required"))
		return
	}

	comp, err := app.Engine.GetFailedCompensation(id)
	if err != nil {
		app.Logger.Error().Err(err).Str("id", id).Msg("Failed to get compensation failure")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.NotExist, err))
		return
	}

	// Verify that the compensation belongs to the project
	if comp.ProjectID != project.ID {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unauthorized, "compensation failure does not belong to this project"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(comp); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Internal, err))
		return
	}
}

// ResolveCompensationFailureHandler marks a failed compensation as resolved
func (app *App) ResolveCompensationFailureHandler(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, "compensation failure ID is required"))
		return
	}

	// Parse request body to get resolution reason
	var request struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, "invalid request body"))
		return
	}

	// Get the compensation failure
	comp, err := app.Engine.GetFailedCompensation(id)
	if err != nil {
		app.Logger.Error().Err(err).Str("id", id).Msg("Failed to get compensation failure")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.NotExist, err))
		return
	}

	// Verify that the compensation belongs to the project
	if comp.ProjectID != project.ID {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unauthorized, "compensation failure does not belong to this project"))
		return
	}

	// Mark the compensation as resolved
	if err := app.Engine.ResolveFailedCompensation(id, request.Reason); err != nil {
		app.Logger.Error().Err(err).Str("id", id).Msg("Failed to resolve compensation failure")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	// Return the updated compensation
	updatedComp, err := app.Engine.GetFailedCompensation(id)
	if err != nil {
		app.Logger.Error().Err(err).Str("id", id).Msg("Failed to get updated compensation failure")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(updatedComp); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Internal, err))
		return
	}
}

// IgnoreCompensationFailureHandler marks a failed compensation as ignored
func (app *App) IgnoreCompensationFailureHandler(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Context().Value(apiKeyContextKey).(string)
	project, err := app.Engine.GetProjectByApiKey(apiKey)
	if err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, err))
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, "compensation failure ID is required"))
		return
	}

	// Parse request body to get ignore reason
	var request struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.InvalidRequest, "invalid request body"))
		return
	}

	// Get the compensation failure
	comp, err := app.Engine.GetFailedCompensation(id)
	if err != nil {
		app.Logger.Error().Err(err).Str("id", id).Msg("Failed to get compensation failure")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.NotExist, err))
		return
	}

	// Verify that the compensation belongs to the project
	if comp.ProjectID != project.ID {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unauthorized, "compensation failure does not belong to this project"))
		return
	}

	// Mark the compensation as ignored
	if err := app.Engine.IgnoreFailedCompensation(id, request.Reason); err != nil {
		app.Logger.Error().Err(err).Str("id", id).Msg("Failed to ignore compensation failure")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	// Return the updated compensation
	updatedComp, err := app.Engine.GetFailedCompensation(id)
	if err != nil {
		app.Logger.Error().Err(err).Str("id", id).Msg("Failed to get updated compensation failure")
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unanticipated, err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(updatedComp); err != nil {
		errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Internal, err))
		return
	}
}
