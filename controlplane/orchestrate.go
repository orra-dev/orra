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
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

func (p *ControlPlane) PrepareOrchestration(projectID string, orchestration *Orchestration, specs []GroundingSpec) {
	orchestration.ID = p.GenerateOrchestrationKey()
	orchestration.Status = Pending
	orchestration.Timestamp = time.Now().UTC()
	orchestration.ProjectID = projectID

	p.orchestrationStoreMu.Lock()
	p.orchestrationStore[orchestration.ID] = orchestration
	p.orchestrationStoreMu.Unlock()

	prepForError := func(orchestration *Orchestration, err error) {
		p.Logger.Error().
			Str("OrchestrationID", orchestration.ID).
			Err(err)
		orchestration.Status = Failed
		orchestration.Timestamp = time.Now().UTC()
		marshaledErr, _ := json.Marshal(err.Error())
		orchestration.Error = marshaledErr
	}

	if err := p.validateWebhook(orchestration.ProjectID, orchestration.Webhook); err != nil {
		prepForError(orchestration, fmt.Errorf("invalid orchestration: %w", err))
		return
	}

	services, err := p.discoverProjectServices(orchestration.ProjectID)
	if err != nil {
		prepForError(orchestration, fmt.Errorf("error discovering services: %w", err))
		return
	}

	serviceDescriptions, err := p.serviceDescriptions(services)
	if err != nil {
		prepForError(orchestration, fmt.Errorf("failed to create service descriptions required for prompting: %w", err))
		return
	}

	actionParams, err := orchestration.Params.Json()
	if err != nil {
		prepForError(orchestration, fmt.Errorf("failed to convert action parameters to prompt friendly format: %w", err))
		return
	}

	var targetGrounding *GroundingSpec
	if len(specs) > 0 {
		targetGrounding = &specs[0]
	}

	callingPlan, err := p.decomposeAction(
		orchestration,
		orchestration.Action.Content,
		actionParams,
		serviceDescriptions,
		targetGrounding,
	)
	if err != nil {
		prepForError(orchestration, fmt.Errorf("error decomposing action: %w", err))
		return
	}

	if err := p.validateActionable(callingPlan.Tasks); err != nil {
		orchestration.Plan = callingPlan
		orchestration.Status = NotActionable
		orchestration.Timestamp = time.Now().UTC()
		marshaledErr, _ := json.Marshal(err.Error())
		orchestration.Error = marshaledErr
		return
	}

	taskZero, onlyServicesCallingPlan := p.callingPlanMinusTaskZero(callingPlan)
	if taskZero == nil {
		orchestration.Plan = callingPlan
		prepForError(orchestration, fmt.Errorf("error locating task zero in calling plan"))
		return
	}

	taskZeroInput, err := json.Marshal(taskZero.Input)
	if err != nil {
		prepForError(orchestration, fmt.Errorf("failed to convert task zero into valid params: %w", err))
		return
	}

	if err = p.validateInput(services, onlyServicesCallingPlan.Tasks); err != nil {
		prepForError(orchestration, fmt.Errorf("error validating plan input/output: %w", err))
		return
	}

	if err := p.addServiceDetails(services, onlyServicesCallingPlan.Tasks); err != nil {
		prepForError(orchestration, fmt.Errorf("error adding service details to calling plan: %w", err))
		return
	}

	orchestration.Plan = onlyServicesCallingPlan
	orchestration.taskZero = taskZeroInput
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
) (*ServiceCallingPlan, error) {
	resp, cachedEntryID, _, err := p.VectorCache.Get(
		context.Background(),
		orchestration.ProjectID,
		action,
		actionParams,
		serviceDescriptions,
		grounding,
	)
	if err != nil {
		return nil, fmt.Errorf("error calling OpenAI API: %v", err)
	}

	var result *ServiceCallingPlan
	sanitisedJSON := sanitizeJSONOutput(resp)
	p.Logger.Debug().
		Str("Sanitized JSON", sanitisedJSON).
		Msg("Service calling plan")

	err = json.Unmarshal([]byte(sanitisedJSON), &result)
	if err != nil {
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		return nil, fmt.Errorf("error parsing LLM response as JSON: %w", err)
	}

	result.ProjectID = orchestration.ProjectID

	return result, nil
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

func (p *ControlPlane) validateInput(services []*ServiceInfo, subTasks []*SubTask) error {
	serviceMap := make(map[string]*ServiceInfo)
	for _, service := range services {
		serviceMap[service.ID] = service
	}

	for _, subTask := range subTasks {
		service, ok := serviceMap[subTask.Service]
		if !ok {
			return fmt.Errorf("service %s not found for subtask %s", subTask.Service, subTask.ID)
		}

		for inputKey := range subTask.Input {
			if !service.Schema.InputIncludes(inputKey) {
				return fmt.Errorf("input %s not supported by service %s for subtask %s", inputKey, subTask.Service, subTask.ID)
			}
		}
	}

	return nil
}

func (p *ControlPlane) addServiceDetails(services []*ServiceInfo, subTasks []*SubTask) error {
	serviceMap := make(map[string]*ServiceInfo)
	for _, service := range services {
		serviceMap[service.ID] = service
	}

	for _, subTask := range subTasks {
		service, ok := serviceMap[subTask.Service]
		if !ok {
			return fmt.Errorf("service %s not found for subtask %s", subTask.Service, subTask.ID)
		}
		subTask.ServiceDetails = service.String()
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

	p.Logger.Debug().
		Fields(struct {
			OrchestrationID string
			ProjectID       string
			Webhook         string
			Payload         string
		}{
			OrchestrationID: orchestration.ID,
			ProjectID:       orchestration.ProjectID,
			Webhook:         orchestration.Webhook,
			Payload:         string(jsonPayload),
		}).
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

func (s ServiceSchema) InputIncludes(src string) bool {
	return s.Input.IncludesProp(src)
}

func (s Spec) IncludesProp(src string) bool {
	_, ok := s.Properties[src]
	return ok
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

func sanitizeJSONOutput(input string) string {
	trimmed := strings.TrimSpace(input)

	if strings.HasPrefix(trimmed, "```json") && strings.HasSuffix(trimmed, "```") {
		withoutStart := strings.TrimPrefix(trimmed, "```json")
		withoutEnd := strings.TrimSuffix(withoutStart, "```")
		return strings.TrimSpace(withoutEnd)
	}

	return input
}
