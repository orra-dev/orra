package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

func (p *ControlPlane) PrepareOrchestration(orchestration *Orchestration) {
	p.orchestrationStoreMu.Lock()
	defer p.orchestrationStoreMu.Unlock()

	p.orchestrationStore[orchestration.ID] = orchestration
	services, err := p.discoverProjectServices(orchestration.ProjectID)
	if err != nil {
		p.Logger.Error().
			Str("OrchestrationID", orchestration.ID).
			Err(fmt.Errorf("error discovering services: %w", err))

		orchestration.Status = Failed
		marshaledErr, _ := json.Marshal(err.Error())
		orchestration.Error = marshaledErr
		return
	}

	callingPlan, err := p.decomposeAction(orchestration, services)
	if err != nil {
		p.Logger.Error().
			Str("OrchestrationID", orchestration.ID).
			Err(fmt.Errorf("error decomposing action: %w", err))

		orchestration.Status = Failed
		marshaledErr, _ := json.Marshal(fmt.Sprintf("Error decomposing action: %s", err.Error()))
		orchestration.Error = marshaledErr
		return
	}

	if err := p.validateActionable(callingPlan.Tasks); err != nil {
		orchestration.Plan = callingPlan
		orchestration.Status = NotActionable
		marshaledErr, _ := json.Marshal(err.Error())
		orchestration.Error = marshaledErr
		return
	}

	taskZero, onlyServicesCallingPlan := p.callingPlanMinusTaskZero(callingPlan)
	if taskZero == nil {
		p.Logger.Error().
			Str("OrchestrationID", orchestration.ID).
			Err(fmt.Errorf("error locating task zero in calling plan"))

		orchestration.Plan = callingPlan
		orchestration.Status = Failed
		marshaledErr, _ := json.Marshal(fmt.Sprintf("Error locating task zero in calling plan"))
		orchestration.Error = marshaledErr
		return
	}

	taskZeroInput, err := json.Marshal(taskZero.Input)
	if err != nil {
		orchestration.Status = Failed
		marshaledErr, _ := json.Marshal(fmt.Sprintf("Failed to convert task zero into valid params: %v", err))
		orchestration.Error = marshaledErr
		return
	}

	if err = p.validateInput(services, onlyServicesCallingPlan.Tasks); err != nil {
		orchestration.Status = Failed
		marshaledErr, _ := json.Marshal(fmt.Sprintf("Error validating plan input/output: %s", err.Error()))
		orchestration.Error = marshaledErr
		return
	}

	if err := p.addServiceDetails(services, onlyServicesCallingPlan.Tasks); err != nil {
		orchestration.Status = Failed
		marshaledErr, _ := json.Marshal(fmt.Sprintf("Error adding service details to calling plan: %s", err.Error()))
		orchestration.Error = marshaledErr
		return
	}

	orchestration.Plan = onlyServicesCallingPlan
	orchestration.taskZero = taskZeroInput
}

func (p *ControlPlane) ExecuteOrchestration(orchestration *Orchestration) {
	p.Logger.Debug().Msgf("About to create Log for orchestration %s", orchestration.ID)
	log := p.LogManager.PrepLogForOrchestration(orchestration.ProjectID, orchestration.ID, orchestration.Plan)

	p.Logger.Debug().Msgf("About to create and start workers for orchestration %s", orchestration.ID)
	p.createAndStartWorkers(orchestration.ID, orchestration.Plan)

	initialEntry := NewLogEntry("task_output", TaskZero, orchestration.taskZero, "control-panel", 0)

	p.Logger.Debug().Msgf("About to append initial entry to Log for orchestration %s", orchestration.ID)
	log.Append(initialEntry)
	orchestration.Status = Processing
}

func (p *ControlPlane) FinalizeOrchestration(
	orchestrationID string,
	status Status,
	reason json.RawMessage,
	results []json.RawMessage) error {
	p.orchestrationStoreMu.Lock()
	defer p.orchestrationStoreMu.Unlock()

	orchestration, exists := p.orchestrationStore[orchestrationID]
	if !exists {
		return fmt.Errorf("control panel cannot finalize missing orchestration %s", orchestrationID)
	}

	orchestration.Status = status
	orchestration.Error = reason
	orchestration.Results = results

	p.Logger.Debug().
		Str("OrchestrationID", orchestration.ID).
		Msgf("About to FinalizeOrchestration with status: %s", orchestration.Status.String())

	p.cleanupLogWorkers(orchestration.ID)

	if err := p.triggerWebhook(orchestration); err != nil {
		return fmt.Errorf("failed to trigger webhook for orchestration %s: %w", orchestration.ID, err)
	}

	return nil
}

func (p *ControlPlane) discoverProjectServices(projectID string) ([]*ServiceInfo, error) {
	p.servicesMu.RLock()
	defer p.servicesMu.RUnlock()

	var out []*ServiceInfo
	projectServices, ok := p.services[projectID]
	if !ok {
		return nil, fmt.Errorf("no services found for project %s", projectID)
	}
	for _, s := range projectServices {
		out = append(out, s)
	}
	return out, nil
}

func (p *ControlPlane) generateLLMPrompt(orchestration *Orchestration, services []*ServiceInfo) (string, error) {
	serviceDescriptions := make([]string, len(services))
	for i, service := range services {
		schemaStr, err := json.Marshal(service.Schema)
		if err != nil {
			return "", fmt.Errorf("failed to marshal service schema: %w", err)
		}
		serviceDescriptions[i] = fmt.Sprintf("Service ID: %s\nService Name: %s\nDescription: %s\nSchema: %s", service.ID, service.Name, service.Description, string(schemaStr))
	}

	actionStr := orchestration.Action.Content

	dataStr, err := json.Marshal(orchestration.Params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data: %w", err)
	}

	prompt := fmt.Sprintf(`You are an AI orchestrator tasked with planning the execution of services based on a user's action. A user's action contains PARAMS for the action to be executed, USE THEM. Your goal is to create an efficient, parallel execution plan that fulfills the user's request.

Available Services:
%s

User Action: %s

Action Params:
%s

Guidelines:
1. Each service described above contains input/output types and description. You must strictly adhere to these types and descriptions when using the services.
2. Each task in the plan should strictly use one of the available services. Follow the JSON conventions for each task.
3. Each task MUST have a unique ID, which is strictly increasing.
4. With the excpetion of Task 0, whose inputs are constants derived from the User Action, inputs for other tasks have to be outputs from preceding tasks. In the latter case, use the format $taskId to denote the ID of the previous task whose output will be the input.
5. There can only be a single Task 0, other tasks HAVE TO CORRESPOND TO AVAILABLE SERVICES.
6. Ensure the plan maximizes parallelizability.
7. Only use the provided services.
	- If a query cannot be addressed using these, USE A "final" TASK TO SUGGEST THE NEXT STEPS.
		- The final task MUST have "final" as the task ID.
		- The final task DOES NOT require a service.
		- The final task input PARAM key should be "error" and the value should explain why the query cannot be addressed.   
		- NO OTHER TASKS ARE REQUIRED. 
8. Never explain the plan with comments.
9. Never introduce new services other than the ones provided.

Please generate a plan in the following JSON format:

{
  "tasks": [
    {
      "id": "task0",
      "input": {
        "param1": "value1"
      }
    },
    {
      "id": "task1",
      "service": "ServiceID",
      "input": {
        "param1": "$task0.param1"
      }
    },
    {
      "id": "task2",
      "service": "AnotherServiceID",
      "input": {
        "param1": "$task1.param1"
      }
    }
  ],
  "parallel_groups": [
    ["task1"],
    ["task2"]
  ]
}

Ensure that the plan is efficient, maximizes parallelization, and accurately fulfills the user's action using the available services. If the action cannot be completed with the given services, explain why in a "final" task and suggest alternatives if possible.

Generate the execution plan:`,
		strings.Join(serviceDescriptions, "\n\n"),
		actionStr,
		string(dataStr),
	)

	return prompt, nil
}

func (p *ControlPlane) decomposeAction(orchestration *Orchestration, services []*ServiceInfo) (*ServiceCallingPlan, error) {
	prompt, err := p.generateLLMPrompt(orchestration, services)
	if err != nil {
		return nil, fmt.Errorf("error generating LLM prompt for decomposing actions: %v", err)
	}

	p.Logger.Debug().
		Str("Prompt", prompt).
		Msg("Decompose action prompt")

	client := openai.NewClient(p.openAIKey)
	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model: openai.GPT4oLatest,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("error calling OpenAI API: %v", err)
	}

	var result *ServiceCallingPlan
	sanitisedJSON := sanitizeJSONOutput(resp.Choices[0].Message.Content)
	p.Logger.Debug().
		Str("Sanitized JSON", sanitisedJSON).
		Msg("Service calling plan")

	err = json.Unmarshal([]byte(sanitisedJSON), &result)
	if err != nil {
		return nil, fmt.Errorf("error parsing LLM response as JSON: %v", err)
	}

	result.ProjectID = orchestration.ProjectID

	return result, nil
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

func (p *ControlPlane) createAndStartWorkers(orchestrationID string, plan *ServiceCallingPlan) {
	p.workerMu.Lock()
	defer p.workerMu.Unlock()

	p.logWorkers[orchestrationID] = make(map[string]context.CancelFunc)

	resultDependencies := make(DependencyKeys)

	for _, task := range plan.Tasks {
		deps := task.extractDependencies()
		resultDependencies[task.ID] = struct{}{}

		p.Logger.Debug().
			Fields(map[string]any{
				"TaskID":          task.ID,
				"Dependencies":    deps,
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
			deps,
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

	if len(resultDependencies) == 0 {
		p.Logger.Error().
			Fields(map[string]any{
				"Dependencies":    resultDependencies,
				"OrchestrationID": orchestrationID,
			}).
			Msg("Result Aggregator has no dependencies")

		return
	}

	p.Logger.Debug().
		Fields(map[string]any{
			"Dependencies":    resultDependencies,
			"OrchestrationID": orchestrationID,
		}).
		Msg("Result Aggregator extracted dependencies")

	aggregator := NewResultAggregator(resultDependencies, p.LogManager)
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
			return fmt.Errorf("%s", subTask.Error)
		}
	}
	return nil
}

func (p *ControlPlane) triggerWebhook(orchestration *Orchestration) error {
	project, ok := p.projects[orchestration.ProjectID]
	if !ok {
		return fmt.Errorf("project %s not found", orchestration.ProjectID)
	}

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
			ProjectID:       project.ID,
			Webhook:         project.Webhook,
			Payload:         string(jsonPayload),
		}).
		Msg("Triggering webhook")

	// Create a new request
	req, err := http.NewRequest("POST", project.Webhook, bytes.NewBuffer(jsonPayload))
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

func (a ActionParams) String() (string, error) {
	data, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	return string(data), nil
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
