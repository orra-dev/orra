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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

func (p *ControlPlane) prepForError(orchestration *Orchestration, err error, status Status) {
	p.Logger.Error().
		Str("OrchestrationID", orchestration.ID).
		Err(err)
	orchestration.Status = status
	orchestration.Timestamp = time.Now().UTC()
	marshaledErr, _ := json.Marshal(err.Error())
	orchestration.Error = marshaledErr
}

func (p *ControlPlane) PrepareOrchestration(projectID string, orchestration *Orchestration, specs []GroundingSpec) error {
	orchestration.ID = p.GenerateOrchestrationKey()
	orchestration.Status = Pending
	orchestration.Timestamp = time.Now().UTC()
	orchestration.ProjectID = projectID

	p.orchestrationStoreMu.Lock()
	p.orchestrationStore[orchestration.ID] = orchestration
	p.orchestrationStoreMu.Unlock()

	if err := p.validateWebhook(orchestration.ProjectID, orchestration.Webhook); err != nil {
		err := fmt.Errorf("invalid orchestration: %w", err)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	services, err := p.discoverProjectServices(orchestration.ProjectID)
	if err != nil {
		err := fmt.Errorf("error discovering services: %w", err)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	serviceDescriptions, err := p.serviceDescriptions(services)
	if err != nil {
		err := fmt.Errorf("failed to create service descriptions required for prompting: %w", err)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	actionParams, err := orchestration.Params.Json()
	if err != nil {
		err := fmt.Errorf("failed to convert action parameters to prompt friendly format: %w", err)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	var targetGrounding *GroundingSpec
	if len(specs) > 0 {
		targetGrounding = &specs[0]
	}

	callingPlan, cachedEntryID, err := p.decomposeAction(
		orchestration,
		orchestration.Action.Content,
		actionParams,
		serviceDescriptions,
		targetGrounding,
	)
	if err != nil {
		err := fmt.Errorf("failed to generate execution plan: %w", err)
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	if err := p.validateActionable(callingPlan.Tasks); err != nil {
		err := fmt.Errorf("failed to generate execution plan: %w", err)
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		p.prepForError(orchestration, err, NotActionable)
		return err
	}

	taskZero, onlyServicesCallingPlan := p.callingPlanMinusTaskZero(callingPlan)
	if taskZero == nil {
		orchestration.Plan = callingPlan
		err := fmt.Errorf("failed to locate task zero in execution plan")
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	taskZeroInput, err := json.Marshal(taskZero.Input)
	if err != nil {
		err := fmt.Errorf("failed to convert task zero in execution plan into valid params: %w", err)
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	if err = p.validateSubTaskInputs(services, onlyServicesCallingPlan.Tasks); err != nil {
		err := fmt.Errorf("execution plan input/output failed validation: %w", err)
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	if err := p.enhanceWithServiceDetails(services, onlyServicesCallingPlan.Tasks); err != nil {
		err := fmt.Errorf("error enhancing execution plan with service details: %w", err)
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	p.Logger.Trace().
		Str("OrchestrationID", orchestration.ID).
		Interface("ServiceCallingPlan", onlyServicesCallingPlan).
		Msg("enhanced service calling plan")

	orchestration.Plan = onlyServicesCallingPlan
	orchestration.taskZero = taskZeroInput

	return nil
}

func (p *ControlPlane) ExecuteOrchestration(orchestration *Orchestration) {
	orchestration.Status = Processing
	orchestration.Timestamp = time.Now().UTC()

	p.Logger.Debug().Msgf("About to create Log for orchestration %s", orchestration.ID)
	log := p.LogManager.PrepLogForOrchestration(orchestration.ProjectID, orchestration.ID, orchestration.Plan)

	p.Logger.Debug().Msgf("About to create and start workers for orchestration %s", orchestration.ID)
	p.createAndStartWorkers(
		orchestration.ID,
		orchestration.Plan,
		orchestration.GetTimeout(),
		orchestration.GetHealthCheckGracePeriod(),
	)

	initialEntry := NewLogEntry("task_output", TaskZero, orchestration.taskZero, "control-panel", 0)

	p.Logger.Debug().Msgf("About to append initial entry to Log for orchestration %s", orchestration.ID)
	log.Append(initialEntry)
}

func (p *ControlPlane) FinalizeOrchestration(
	orchestrationID string,
	status Status,
	reason json.RawMessage,
	results []json.RawMessage,
	skipWebhook bool,
) error {
	p.orchestrationStoreMu.Lock()
	defer p.orchestrationStoreMu.Unlock()

	orchestration, exists := p.orchestrationStore[orchestrationID]
	if !exists {
		return fmt.Errorf("control plane cannot finalize missing orchestration %s", orchestrationID)
	}

	orchestration.Status = status
	orchestration.Timestamp = time.Now().UTC()
	orchestration.Error = reason
	orchestration.Results = results

	p.Logger.Debug().
		Str("OrchestrationID", orchestration.ID).
		Msgf("About to FinalizeOrchestration with status: %s", orchestration.Status.String())

	if !skipWebhook {
		if err := p.triggerWebhook(orchestration); err != nil {
			return fmt.Errorf("failed to trigger webhook for orchestration %s: %w", orchestration.ID, err)
		}
	}

	p.cleanupLogWorkers(orchestration.ID)

	return nil
}

func (p *ControlPlane) serviceDescriptions(services []*ServiceInfo) (string, error) {
	out := make([]string, len(services))
	for i, service := range services {
		schemaStr, err := json.Marshal(service.Schema)
		if err != nil {
			return "", fmt.Errorf("failed to marshal service schema: %w", err)
		}
		out[i] = fmt.Sprintf("Service ID: %s\nService Name: %s\nDescription: %s\nSchema: %s", service.ID, service.Name, service.Description, string(schemaStr))
	}
	return strings.Join(out, "\n\n"), nil
}

func (p *ControlPlane) discoverProjectServices(projectID string) ([]*ServiceInfo, error) {
	p.servicesMu.RLock()
	defer p.servicesMu.RUnlock()

	projectServices, ok := p.services[projectID]
	if !ok {
		return nil, fmt.Errorf("no services found for project %s", projectID)
	}

	out := make([]*ServiceInfo, 0, len(projectServices))
	for _, s := range projectServices {
		out = append(out, s)
	}

	slices.SortFunc(out, func(a, b *ServiceInfo) int {
		return strings.Compare(a.ID, b.ID)
	})

	return out, nil
}

func (p *ControlPlane) decomposeAction(
	orchestration *Orchestration,
	action string,
	actionParams json.RawMessage,
	serviceDescriptions string,
	grounding *GroundingSpec,
) (*ServiceCallingPlan, string, error) {
	planJson, cachedEntryID, _, err := p.VectorCache.Get(
		context.Background(),
		orchestration.ProjectID,
		action,
		actionParams,
		serviceDescriptions,
		grounding,
	)
	if err != nil {
		return nil, cachedEntryID, err
	}

	var result *ServiceCallingPlan
	if err = json.Unmarshal([]byte(planJson), &result); err != nil {
		return nil, cachedEntryID, fmt.Errorf("error parsing LLM response as JSON: %w", err)
	}

	for i := 0; i < len(result.Tasks); i++ {
		result.Tasks[i].Service = strings.ToLower(result.Tasks[i].Service)
	}

	result.ProjectID = orchestration.ProjectID

	return result, cachedEntryID, nil
}

func (p *ControlPlane) validateWebhook(projectID string, webhookUrl string) error {
	if len(strings.TrimSpace(webhookUrl)) == 0 {
		return fmt.Errorf("a webhook url is required to return orchestration results")
	}

	if _, err := url.ParseRequestURI(webhookUrl); err != nil {
		return fmt.Errorf("webhook url %s is not valid: %w", webhookUrl, err)
	}

	project := p.projects[projectID]
	if !contains(project.Webhooks, webhookUrl) {
		return fmt.Errorf("webhook url %s not found in project %s", webhookUrl, projectID)
	}

	return nil
}

// validateSubTaskInputs checks that each subTask's input keys are valid for its service
// and that every required key provided by the service is present in the subTask.
func (p *ControlPlane) validateSubTaskInputs(services []*ServiceInfo, subTasks []*SubTask) error {
	// Build a lookup map for services.
	serviceMap := make(map[string]*ServiceInfo, len(services))
	for _, service := range services {
		serviceMap[service.ID] = service
	}

	var validationErrors []error

	// Process each subTask independently.
	for _, subTask := range subTasks {
		svc, ok := serviceMap[subTask.Service]
		if !ok {
			validationErrors = append(validationErrors,
				fmt.Errorf("service %s not found for subtask %s", subTask.Service, subTask.ID))
			continue
		}

		// Create a set of expected keys.
		expectedKeys := svc.InputPropKeys()
		expectedSet := make(map[string]struct{}, len(expectedKeys))
		for _, key := range expectedKeys {
			expectedSet[key] = struct{}{}
		}

		// Check that every input provided is allowed.
		for inputKey := range subTask.Input {
			if _, ok := expectedSet[inputKey]; !ok {
				validationErrors = append(validationErrors,
					fmt.Errorf("input %s not supported by service %s for subtask %s", inputKey, svc.ID, subTask.ID))
			}
		}

		// Check that every expected key is present.
		for _, key := range expectedKeys {
			if _, present := subTask.Input[key]; !present {
				validationErrors = append(validationErrors,
					fmt.Errorf("service %s is missing required input %s in subtask %s", svc.ID, key, subTask.ID))
			}
		}
	}

	if len(validationErrors) > 0 {
		// Use errors.Join to compose a single error that wraps all the individual errors.
		return fmt.Errorf("input validation errors: %w", errors.Join(validationErrors...))
	}

	return nil
}

func (s *SubTask) InputKeys() []string {
	var out []string
	for k := range s.Input {
		out = append(out, k)
	}
	return out
}

func (p *ControlPlane) enhanceWithServiceDetails(services []*ServiceInfo, subTasks []*SubTask) error {
	serviceMap := make(map[string]*ServiceInfo)
	for _, service := range services {
		serviceMap[service.ID] = service
	}

	for _, subTask := range subTasks {
		service, ok := serviceMap[subTask.Service]
		if !ok {
			return fmt.Errorf("service %s not found for subtask %s", subTask.Service, subTask.ID)
		}
		subTask.ServiceName = service.Name
		subTask.Capabilities = []string{service.Description}
		subTask.ExpectedInput = service.Schema.Input
		subTask.ExpectedOutput = service.Schema.Output
	}

	return nil
}

func (p *ControlPlane) createAndStartWorkers(
	orchestrationID string,
	plan *ServiceCallingPlan,
	taskTimeout,
	healthCheckGracePeriod time.Duration,
) {
	p.workerMu.Lock()
	defer p.workerMu.Unlock()

	p.logWorkers[orchestrationID] = make(map[string]context.CancelFunc)

	resultAggregatorDeps := make(DependencyKeySet)

	for _, task := range plan.Tasks {
		taskDeps := task.extractDependencies()
		resultAggregatorDeps[task.ID] = struct{}{}

		p.Logger.Debug().
			Fields(map[string]any{
				"TaskID":          task.ID,
				"Dependencies":    taskDeps,
				"OrchestrationID": orchestrationID,
			}).
			Msg("Task extracted dependencies")

		service, err := p.GetServiceByID(task.Service)
		if err != nil {
			p.Logger.Error().Err(err).
				Str("taskID", task.ID).
				Str("ServiceID", task.Service).
				Msg("Failed to get service for task")
			return
		}

		worker := NewTaskWorker(
			service,
			task.ID,
			taskDeps,
			taskTimeout,
			healthCheckGracePeriod,
			p.LogManager,
		)
		ctx, cancel := context.WithCancel(context.Background())
		p.logWorkers[orchestrationID][task.ID] = cancel
		p.Logger.Debug().
			Fields(struct {
				TaskID          string
				OrchestrationID string
			}{
				TaskID:          task.ID,
				OrchestrationID: orchestrationID,
			}).
			Msg("Starting worker for task")

		go worker.Start(ctx, orchestrationID)
	}

	if len(resultAggregatorDeps) == 0 {
		p.Logger.Error().
			Fields(map[string]any{
				"Dependencies":    resultAggregatorDeps,
				"OrchestrationID": orchestrationID,
			}).
			Msg("Result Aggregator has no dependencies")

		return
	}

	p.Logger.Debug().
		Fields(map[string]any{
			"Dependencies":    resultAggregatorDeps,
			"OrchestrationID": orchestrationID,
		}).
		Msg("Result Aggregator extracted dependencies")

	aggregator := NewResultAggregator(resultAggregatorDeps, p.LogManager)
	ctx, cancel := context.WithCancel(context.Background())
	p.logWorkers[orchestrationID][ResultAggregatorID] = cancel

	fTracker := NewFailureTracker(p.LogManager)
	fCtx, fCancel := context.WithCancel(context.Background())
	p.logWorkers[orchestrationID][FailureTrackerID] = fCancel

	p.Logger.Debug().Str("orchestrationID", orchestrationID).Msg("Starting result aggregator for orchestration")
	go aggregator.Start(ctx, orchestrationID)

	p.Logger.Debug().Str("orchestrationID", orchestrationID).Msg("Starting failure tracker for orchestration")
	go fTracker.Start(fCtx, orchestrationID)
}

func (p *ControlPlane) cleanupLogWorkers(orchestrationID string) {
	p.workerMu.Lock()
	defer p.workerMu.Unlock()

	if cancelFns, exists := p.logWorkers[orchestrationID]; exists {
		for logWorker, cancel := range cancelFns {
			p.Logger.Debug().
				Str("LogWorker", logWorker).
				Msgf("Stopping Log worker for orchestration: %s", orchestrationID)

			cancel() // This will trigger ctx.Done() in the worker
		}
		delete(p.logWorkers, orchestrationID)
		p.Logger.Debug().
			Str("OrchestrationID", orchestrationID).
			Msg("Cleaned up task workers for orchestration.")
	}
}

func (p *ControlPlane) callingPlanMinusTaskZero(callingPlan *ServiceCallingPlan) (*SubTask, *ServiceCallingPlan) {
	var taskZero *SubTask
	var serviceTasks []*SubTask

	for _, subTask := range callingPlan.Tasks {
		if strings.EqualFold(subTask.ID, TaskZero) {
			taskZero = subTask
			continue
		}
		serviceTasks = append(serviceTasks, subTask)
	}

	return taskZero, &ServiceCallingPlan{
		ProjectID:      callingPlan.ProjectID,
		Tasks:          serviceTasks,
		ParallelGroups: callingPlan.ParallelGroups,
	}
}

func (p *ControlPlane) validateActionable(subTasks []*SubTask) error {
	for _, subTask := range subTasks {
		if strings.EqualFold(subTask.ID, "final") {
			return fmt.Errorf("%s", subTask.Input["error"])
		}
	}
	return nil
}

func (p *ControlPlane) triggerWebhook(orchestration *Orchestration) error {
	var payload = struct {
		OrchestrationID string            `json:"orchestrationId"`
		Results         []json.RawMessage `json:"results"`
		Status          Status            `json:"status"`
		Error           json.RawMessage   `json:"error,omitempty"`
	}{
		OrchestrationID: orchestration.ID,
		Results:         orchestration.Results,
		Status:          orchestration.Status,
		Error:           orchestration.Error,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to trigger webhook failed to marshal payload: %w", err)
	}

	p.Logger.Trace().
		Str("ProjectID", orchestration.ProjectID).
		Str("OrchestrationID", orchestration.ID).
		Str("Webhook", orchestration.Webhook).
		RawJSON("Payload", jsonPayload).
		Msg("Triggering webhook")

	// Create a new request
	req, err := http.NewRequest("POST", orchestration.Webhook, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Orra/1.0")

	// Create an HTTP client with a timeout
	client := &http.Client{
		Timeout: time.Second * 10,
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			p.Logger.Error().
				Str("OrchestrationID", orchestration.ID).
				Err(fmt.Errorf("failed to close response body when triggering Webhook: %w", err))
		}
	}(resp.Body)

	// Check the response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (s Spec) String() (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (p ActionParams) Json() (json.RawMessage, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (si *ServiceInfo) String() string {
	return fmt.Sprintf("[%s] %s - %s", si.Type.String(), si.Name, si.Description)
}

func (si *ServiceInfo) InputPropKeys() []string {
	var out []string
	for k := range si.Schema.Input.Properties {
		out = append(out, k)
	}
	return out
}

func extractValidJSONOutput(input string) (string, error) {
	// Define the markers to locate the JSON block.
	startMarker := "```json"
	endMarker := "```"

	// Find the start of the JSON block.
	startIdx := strings.Index(input, startMarker)
	if startIdx == -1 {
		// Return full input if the JSON start marker is not found.
		return "", fmt.Errorf("cannot find opening JSON marker in %s", input)
	}

	// Start after the marker.
	startIdx += len(startMarker)

	// Find the closing marker after the start.
	endIdx := strings.Index(input[startIdx:], endMarker)
	if endIdx == -1 {
		// Return full input if no closing marker is found.
		return "", fmt.Errorf("cannot find closing JSON marker in %s", input)
	}

	var temp map[string]any
	jsonContent := strings.TrimSpace(input[startIdx : startIdx+endIdx])
	if err := json.Unmarshal([]byte(jsonContent), &temp); err != nil {
		return "", fmt.Errorf("cannot parse invalid JSON: %s", input)
	}

	return jsonContent, nil
}
