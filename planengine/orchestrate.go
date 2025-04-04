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
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	back "github.com/cenkalti/backoff/v4"
)

const (
	maxPreparationRetries = 2
)

type PreparationError struct {
	Status Status
	Err    error
}

func (e PreparationError) Error() string {
	return fmt.Sprintf("preparation failed with status %s: %v", e.Status, e.Err)
}

func (p *PlanEngine) prepForError(orchestration *Orchestration, err error, status Status) {
	p.Logger.Error().
		Str("OrchestrationID", orchestration.ID).
		Err(err)
	orchestration.Status = status
	orchestration.Timestamp = time.Now().UTC()
	marshaledErr, _ := json.Marshal(err.Error())
	orchestration.Error = marshaledErr

	if storeErr := p.orchestrationStorage.StoreOrchestration(orchestration); storeErr != nil {
		p.Logger.Error().
			Err(storeErr).
			Str("OrchestrationID", orchestration.ID).
			Msg("Failed to persist failed orchestration state")
	}
}

func (p *PlanEngine) InjectGroundingMatchForAnyAppliedSpecs(ctx context.Context, orchestration *Orchestration, specs []GroundingSpec) error {
	if len(specs) == 0 {
		return nil
	}

	hit, matchScore, err := orchestration.MatchingGroundingAgainstAction(ctx, p.SimilarityMatcher, specs)
	if err != nil {
		return fmt.Errorf("cannot match grounding specs against orchestration action: %w", err)
	}

	orchestration.GroundingHit = hit

	p.Logger.Trace().
		Str("ProjectID", orchestration.ProjectID).
		Str("OrchestrationID", orchestration.ID).
		Str("OrchestrationAction", normalizeActionPattern(orchestration.Action.Content)).
		Str("MatchedGroundingUseCaseAction", normalizeActionPattern(orchestration.Action.Content)).
		Float64("matchScore", matchScore).
		Msg("Matched grounding to orchestration")

	return nil
}

func (p *PlanEngine) PrepareOrchestration(ctx context.Context, projectID string, orchestration *Orchestration, specs []GroundingSpec) error {
	// Initial setup and validation that shouldn't be retried
	orchestration.ID = p.GenerateOrchestrationKey()
	orchestration.Status = Pending
	orchestration.Timestamp = time.Now().UTC()
	orchestration.ProjectID = projectID

	p.orchestrationStoreMu.Lock()
	p.orchestrationStore[orchestration.ID] = orchestration
	p.orchestrationStoreMu.Unlock()

	// Persist to storage
	if err := p.orchestrationStorage.StoreOrchestration(orchestration); err != nil {
		p.Logger.Error().
			Err(err).
			Str("OrchestrationID", orchestration.ID).
			Msg("Failed to persist orchestration")
		return fmt.Errorf("failed to persist orchestration: %w", err)
	}

	// Non-retryable validations
	if err := p.validateActionParams(orchestration.Params); err != nil {
		err = fmt.Errorf("invalid orchestration: %w", err)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	if err := p.validateWebhook(orchestration.ProjectID, orchestration.Webhook); err != nil {
		err = fmt.Errorf("invalid orchestration: %w", err)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	services, err := p.discoverProjectServices(orchestration.ProjectID)
	if err != nil {
		err = fmt.Errorf("error discovering services: %w", err)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	serviceDescriptions, err := p.serviceDescriptions(services)
	if err != nil {
		err = fmt.Errorf("failed to create service descriptions: %w", err)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	actionParams, err := orchestration.Params.Json()
	if err != nil {
		err = fmt.Errorf("failed to convert action parameters: %w", err)
		p.prepForError(orchestration, err, Failed)
		return err
	}

	if err := p.InjectGroundingMatchForAnyAppliedSpecs(ctx, orchestration, specs); err != nil {
		p.prepForError(orchestration, err, Failed)
		return err
	}

	// Configure exponential backoff for retryable operations
	expBackoff := back.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = 10 * time.Second
	expBackoff.Multiplier = 2.0
	expBackoff.RandomizationFactor = 0.1
	expBackoff.MaxElapsedTime = 30 * time.Second
	expBackoff.Reset()

	var consecutiveRetries int
	var errorFeedback string

	// Setup retry operation
	operation := func() error {
		// Attempt the retryable portion of execution plan preparation with current retry count
		err := p.attemptRetryablePreparation(
			ctx,
			orchestration,
			services,
			actionParams,
			serviceDescriptions,
			errorFeedback,
			consecutiveRetries,
		)
		if err == nil {
			return nil
		}

		// Handle different error types
		var prepErr PreparationError
		if errors.As(err, &prepErr) {
			switch prepErr.Status {
			case NotActionable:
				// NotActionable errors are permanent
				return back.Permanent(prepErr)

			case FailedNotRetryable:
				// FailedNotRetryable errors are permanent
				return back.Permanent(prepErr)

			case Failed:
				if consecutiveRetries > 0 {
					type multiError interface {
						Unwrap() []error
					}
					var mErr multiError
					if errors.As(prepErr.Err, &mErr) {
						errorFeedback = "Validation Errors:\n"
						for _, vErr := range mErr.Unwrap() {
							errorFeedback += fmt.Sprintf("- %s\n", vErr)
						}
						p.Logger.Trace().Str("errorFeedback", errorFeedback).Msg("multiError")
					} else {
						errorFeedback = prepErr.Err.Error()
						p.Logger.Trace().Str("errorFeedback", errorFeedback).Msg("singleError")
					}
				}

				// Check retry count
				if consecutiveRetries < maxPreparationRetries {
					consecutiveRetries++
					return prepErr
				}
				p.Logger.Error().
					Err(err).
					Str("OrchestrationID", orchestration.ID).
					Msg("Failed to persist orchestration")
				return back.Permanent(fmt.Errorf("exceeded maximum retries (%d): %w", maxPreparationRetries, prepErr.Err))

			default:
				return back.Permanent(prepErr)
			}
		}

		// Non-PreparationError errors are permanent
		return back.Permanent(err)
	}

	// Execute the retry operation with notifications
	err = back.RetryNotify(operation, expBackoff, func(err error, duration time.Duration) {
		p.Logger.Info().
			Err(err).
			Str("orchestrationID", orchestration.ID).
			Dur("retryAfter", duration).
			Int("retryAttempt", consecutiveRetries).
			Msg("Retrying orchestration preparation")
	})

	if err != nil {
		var prepErr PreparationError
		if errors.As(err, &prepErr) {
			p.prepForError(orchestration, prepErr.Err, prepErr.Status)
		} else {
			p.prepForError(orchestration, err, Failed)
		}
		return err
	}

	return nil
}

func (p *PlanEngine) attemptRetryablePreparation(ctx context.Context, orchestration *Orchestration, services []*ServiceInfo, actionParams json.RawMessage, serviceDescriptions string, retryCauseIfAny string, retryCount int) error {
	p.Logger.Trace().Str("retryCauseIfAny", retryCauseIfAny).Msg("")

	callingPlan, cachedEntryID, isCacheHit, err := p.decomposeAction(
		ctx,
		orchestration,
		orchestration.Action.Content,
		actionParams,
		serviceDescriptions,
		retryCauseIfAny,
	)
	if err != nil {
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		return PreparationError{Status: Failed, Err: fmt.Errorf("failed to generate execution plan: %w", err)}
	}

	// Validate actionable - NotActionable is a permanent error
	if err := p.validateActionable(callingPlan.Tasks); err != nil {
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		return PreparationError{Status: NotActionable, Err: err}
	}

	// Process and validate the rest of the plan
	taskZero, onlyServicesCallingPlan := p.callingPlanMinusTaskZero(callingPlan)
	if taskZero == nil {
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		return PreparationError{Status: Failed, Err: fmt.Errorf("task zero should be in execution plan but was not located")}
	}

	taskZeroInput, err := json.Marshal(taskZero.Input)
	if err != nil {
		err := fmt.Errorf("failed to convert task zero to raw JSON so it can be used as the initial log entry in audit logs: %w", err)
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		return PreparationError{Status: Failed, Err: err}
	}

	if !isCacheHit {
		// Validate that all action parameters are properly included in TaskZero
		if status, err := p.validateTaskZeroParams(callingPlan, actionParams, retryCount); err != nil {
			p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
			return PreparationError{
				Status: status,
				Err:    fmt.Errorf("execution plan action parameters validation failed: %w", err),
			}
		}

		if status, err := p.validateNoCompositeTaskZeroRefs(callingPlan, retryCount); err != nil {
			p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
			return PreparationError{
				Status: status,
				Err:    fmt.Errorf("execution plan contains invalid composite task0 references: %w", err),
			}
		}

		// Validate subtask inputs
		if err = p.validateSubTaskInputs(services, onlyServicesCallingPlan.Tasks); err != nil {
			p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
			// This error might contain multiple validation errors joined together
			return PreparationError{Status: Failed, Err: fmt.Errorf("execution plan input/output failed validation: %w", err)}
		}
	}

	// Enhance with service details
	if err := p.enhanceWithServiceDetails(services, callingPlan.Tasks); err != nil {
		p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
		return PreparationError{Status: Failed, Err: fmt.Errorf("error enhancing execution plan with service details: %w", err)}
	}

	if !isCacheHit {
		if err := p.validateExecPlanAgainstDomain(ctx, callingPlan, orchestration); err != nil {
			p.VectorCache.Remove(orchestration.ProjectID, cachedEntryID)
			return fmt.Errorf("execution plan is invalid against domain: %w", err)
		}
	}

	// Store the final plan
	orchestration.Plan = onlyServicesCallingPlan
	orchestration.TaskZero = taskZeroInput
	return nil
}

func (p *PlanEngine) ExecuteOrchestration(ctx context.Context, orchestration *Orchestration) {
	p.Logger.Debug().Msgf("About to create Log for orchestration %s", orchestration.ID)
	log := p.LogManager.PrepLogForOrchestration(orchestration.ProjectID, orchestration.ID, orchestration.Plan)

	orchestration.Status = Processing
	orchestration.Timestamp = time.Now().UTC()

	if err := p.orchestrationStorage.StoreOrchestration(orchestration); err != nil {
		p.Logger.Error().
			Err(err).
			Str("OrchestrationID", orchestration.ID).
			Msg("Failed to persist orchestration")
	}

	p.Logger.Debug().Msgf("About to create and start workers for orchestration %s", orchestration.ID)
	p.createAndStartWorkers(
		ctx,
		orchestration.ID,
		orchestration.Plan,
		orchestration.GetTimeout(),
		orchestration.GetHealthCheckGracePeriod(),
	)

	initialEntry := NewLogEntry("task_output", TaskZero, orchestration.TaskZero, "control-panel", 0)
	log.Append(orchestration.ID, initialEntry, true)
	p.Logger.
		Trace().
		Str("OrchestrationID", orchestration.ID).
		Interface("InitialEntry", initialEntry).
		Msg("Appended initial entry to Log")
}

func (p *PlanEngine) FinalizeOrchestration(
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
		return fmt.Errorf("plan engine cannot finalize missing orchestration %s", orchestrationID)
	}

	orchestration.Status = status
	orchestration.Timestamp = time.Now().UTC()
	orchestration.Error = reason
	orchestration.Results = results

	// Persist updated state
	if err := p.orchestrationStorage.StoreOrchestration(orchestration); err != nil {
		p.Logger.Error().
			Err(err).
			Str("OrchestrationID", orchestration.ID).
			Msg("Failed to persist orchestration state")
		return fmt.Errorf("failed to persist orchestration state: %w", err)
	}

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

func (p *PlanEngine) CancelOrchestration(orchestrationID string, reason json.RawMessage) error {
	p.orchestrationStoreMu.Lock()
	defer p.orchestrationStoreMu.Unlock()

	orchestration, exists := p.orchestrationStore[orchestrationID]
	if !exists {
		return fmt.Errorf("plan engine cannot cancel missing orchestration %s", orchestrationID)
	}

	orchestration.Status = Cancelled
	orchestration.Timestamp = time.Now().UTC()
	orchestration.Error = reason

	// Persist updated state
	if err := p.orchestrationStorage.StoreOrchestration(orchestration); err != nil {
		return fmt.Errorf("failed to persist orchestration state: %w", err)
	}

	p.Logger.Debug().
		Str("OrchestrationID", orchestration.ID).
		Msgf("About to Cancel Orchestration with status: %s", orchestration.Status.String())

	return nil
}

func (p *PlanEngine) CancelAnyActiveOrchestrations() error {
	candidates := p.getAllActiveOrchestrations()
	if len(candidates) == 0 {
		return nil
	}

	var errs []error
	reason, _ := json.Marshal(PlanEngineShuttingDownErrCode)
	for _, o := range candidates {
		p.LogManager.MarkOrchestration(o.ID, Cancelled, []byte(PlanEngineShuttingDownErrCode))

		if err := p.CancelOrchestration(o.ID, reason); err != nil {
			errs = append(errs, err)
			p.Logger.Trace().Str("OrchestrationID", o.ID).Str("Status", o.Status.String()).Msg("failed to cancel")
			continue
		}

		p.Logger.Trace().Str("OrchestrationID", o.ID).Str("Status", o.Status.String()).Msg("finalised and cancelled")
	}

	if len(errs) > 0 {
		if len(errs) == len(candidates) {
			return fmt.Errorf("all orchestrations failed to cancel: %w", errors.Join(errs...))
		}
		return fmt.Errorf("some orchestrations failed to cancel: %w", errors.Join(errs...))
	}
	return nil
}

func (p *PlanEngine) getAllActiveOrchestrations() []*Orchestration {
	p.orchestrationStoreMu.RLock()
	defer p.orchestrationStoreMu.RUnlock()

	var result []*Orchestration
	for _, o := range p.orchestrationStore {
		if o.Status == Processing {
			result = append(result, o)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	p.Logger.Trace().Interface("ActiveOrchestrations", result).Msg("")
	return result
}

func (p *PlanEngine) serviceDescriptions(services []*ServiceInfo) (string, error) {
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

func (p *PlanEngine) discoverProjectServices(projectID string) ([]*ServiceInfo, error) {
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

func (p *PlanEngine) decomposeAction(ctx context.Context, orchestration *Orchestration, action string, actionParams json.RawMessage, serviceDescriptions string, retryCauseIfAny string) (*ExecutionPlan, string, bool, error) {
	cacheResult, _, err := p.VectorCache.Get(
		ctx,
		orchestration.ProjectID,
		action,
		actionParams,
		serviceDescriptions,
		orchestration.GroundingHit,
		retryCauseIfAny,
	)
	if err != nil {
		return nil, "", false, err
	}

	var result *ExecutionPlan
	if err = json.Unmarshal([]byte(cacheResult.Response), &result); err != nil {
		return nil, cacheResult.ID, false, fmt.Errorf("error parsing LLM response as JSON: %w", err)
	}

	for i := 0; i < len(result.Tasks); i++ {
		result.Tasks[i].Service = strings.ToLower(result.Tasks[i].Service)
	}

	result.ProjectID = orchestration.ProjectID
	result.GroundingHit = orchestration.GroundingHit

	return result, cacheResult.ID, cacheResult.Hit, nil
}

func (p *PlanEngine) validateActionParams(params ActionParams) error {
	if len(params) == 0 {
		return nil
	}

	if err := ValidateActionParams(params); err != nil {
		return fmt.Errorf("action parameters invalid: %w", err)
	}

	return nil
}

func (p *PlanEngine) validateWebhook(projectID string, webhookUrl string) error {
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
func (p *PlanEngine) validateSubTaskInputs(services []*ServiceInfo, subTasks []*SubTask) error {
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

func (p *PlanEngine) validateExecPlanAgainstDomain(ctx context.Context, plan *ExecutionPlan, orchestration *Orchestration) error {
	// Skip validation if no grounding was used
	if plan.GroundingHit == nil {
		return nil
	}

	orchestratedAction := orchestration.Action.Content
	generator := NewPddlGenerator(orchestratedAction, plan, p.SimilarityMatcher, p.Logger)

	domain, err := generator.GenerateDomain(ctx)
	if err != nil {
		return fmt.Errorf("failed to generate PDDL domain: %w", err)
	}

	problem, err := generator.GenerateProblem(ctx)
	if err != nil {
		return fmt.Errorf("failed to generate PDDL domain: %w", err)
	}

	p.Logger.Trace().
		Str("ProjectID", orchestration.ProjectID).
		Str("OrchestrationID", orchestration.ID).
		Str("Action", orchestratedAction).
		Str("Domain", domain).
		Msg("Generate PDDL domain")

	p.Logger.Trace().
		Str("ProjectID", orchestration.ProjectID).
		Str("OrchestrationID", orchestration.ID).
		Str("Action", orchestratedAction).
		Str("Domain", problem).
		Msg("Generate PDDL problem")

	// Validate using VAL
	if err := p.PddlValidator.Validate(ctx, orchestration.ProjectID, domain, problem); err != nil {
		var valErr *PddlValidationError
		if errors.As(err, &valErr) {
			return fmt.Errorf("PDDL validation failed: %w", valErr)
		}
		return fmt.Errorf("PDDL validation failed: %w", err)
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

func (p *PlanEngine) enhanceWithServiceDetails(services []*ServiceInfo, subTasks []*SubTask) error {
	serviceMap := make(map[string]*ServiceInfo)
	for _, service := range services {
		serviceMap[service.ID] = service
	}

	for _, subTask := range subTasks {
		if strings.EqualFold(subTask.ID, TaskZero) {
			continue
		}

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

func (p *PlanEngine) createAndStartWorkers(ctx context.Context, orchestrationID string, plan *ExecutionPlan, taskTimeout, healthCheckGracePeriod time.Duration) {
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
		taskCtx, cancel := context.WithCancel(ctx)
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

		go worker.Start(taskCtx, orchestrationID)
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
	aggCtx, cancel := context.WithCancel(ctx)
	p.logWorkers[orchestrationID][ResultAggregatorID] = cancel

	fTracker := NewFailureTracker(p.LogManager)
	fCtx, fCancel := context.WithCancel(ctx)
	p.logWorkers[orchestrationID][FailureTrackerID] = fCancel

	p.Logger.Debug().Str("orchestrationID", orchestrationID).Msg("Starting result aggregator for orchestration")
	go aggregator.Start(aggCtx, orchestrationID)

	p.Logger.Debug().Str("orchestrationID", orchestrationID).Msg("Starting failure tracker for orchestration")
	go fTracker.Start(fCtx, orchestrationID)
}

func (p *PlanEngine) cleanupLogWorkers(orchestrationID string) {
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

func (p *PlanEngine) callingPlanMinusTaskZero(callingPlan *ExecutionPlan) (*SubTask, *ExecutionPlan) {
	var taskZero *SubTask
	var serviceTasks []*SubTask

	// First pass: Find TaskZero and service tasks
	for _, subTask := range callingPlan.Tasks {
		if strings.EqualFold(subTask.ID, TaskZero) {
			taskZero = subTask
			continue
		}
		serviceTasks = append(serviceTasks, subTask)
	}

	// If no TaskZero found, return early
	if taskZero == nil {
		return nil, &ExecutionPlan{
			ProjectID:      callingPlan.ProjectID,
			Tasks:          serviceTasks,
			ParallelGroups: callingPlan.ParallelGroups,
		}
	}

	// Ensure TaskZero has an Input map
	if taskZero.Input == nil {
		taskZero.Input = make(map[string]any)
	}

	// Track which keys we've seen and their corresponding values in taskZero
	keyCounter := make(map[string]int)

	// Process each service task looking for direct values
	for _, task := range serviceTasks {
		for inputKey, inputVal := range task.Input {
			// Skip if it's already a reference
			strVal, isString := inputVal.(string)
			if isString && strings.HasPrefix(strVal, "$") {
				continue
			}

			// Check if we've already seen this key
			baseKey := inputKey

			// If this key already exists in TaskZero with different value,
			// we need to use a numbered variant
			//needsUniqueKey := false

			// First occurrence - use the key as is
			if keyCounter[baseKey] == 0 {
				taskZero.Input[baseKey] = inputVal
				keyCounter[baseKey] = 1
				task.Input[inputKey] = fmt.Sprintf("$%s.%s", TaskZero, baseKey)
			} else {
				// Check if any existing value in taskZero matches this value
				foundMatch := false
				var matchingKey string

				// Look for a matching value in taskZero
				for tKey, tVal := range taskZero.Input {
					// Only check keys with the same base
					if tKey == baseKey || strings.HasPrefix(tKey, baseKey+"_") {
						// If values are equal, we can reuse this key
						if reflect.DeepEqual(tVal, inputVal) {
							foundMatch = true
							matchingKey = tKey
							break
						}
					}
				}

				if foundMatch {
					// Reference the existing key in taskZero
					task.Input[inputKey] = fmt.Sprintf("$%s.%s", TaskZero, matchingKey)
				} else {
					// Create a new unique key
					keyCounter[baseKey]++
					uniqueKey := fmt.Sprintf("%s_%d", baseKey, keyCounter[baseKey])

					// Add to TaskZero with the unique key
					taskZero.Input[uniqueKey] = inputVal

					// Update task to reference the unique key
					task.Input[inputKey] = fmt.Sprintf("$%s.%s", TaskZero, uniqueKey)
				}
			}
		}
	}

	return taskZero, &ExecutionPlan{
		ProjectID:      callingPlan.ProjectID,
		Tasks:          serviceTasks,
		ParallelGroups: callingPlan.ParallelGroups,
	}
}

func (p *PlanEngine) validateActionable(subTasks []*SubTask) error {
	for _, subTask := range subTasks {
		if strings.EqualFold(subTask.ID, "final") {
			return fmt.Errorf("%s", subTask.Input["error"])
		}
	}
	return nil
}

func (p *PlanEngine) triggerWebhook(orchestration *Orchestration) error {
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

func (o *Orchestration) MatchingGroundingAgainstAction(ctx context.Context, matcher SimilarityMatcher, specs []GroundingSpec) (*GroundingHit, float64, error) {
	for _, spec := range specs {
		for _, useCase := range spec.UseCases {
			normalizedPlanAction := normalizeActionPattern(o.Action.Content)
			normalizedUseCase := normalizeActionPattern(useCase.Action)

			hasMatch, score, err := matcher.MatchTexts(ctx, normalizedPlanAction, normalizedUseCase, GroundingThreshold)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to match action: %w", err)
			}

			if !hasMatch {
				continue
			}
			return &GroundingHit{
				Name:        spec.Name,
				Domain:      spec.Domain,
				Version:     spec.Version,
				UseCase:     useCase,
				Constraints: spec.Constraints,
			}, score, nil
		}
	}

	return nil, 0, fmt.Errorf("no matching use-case found")
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
	// Find the start of the JSON block.
	startIdx := strings.Index(input, StartJsonMarker)
	if startIdx == -1 {
		// Return full input if the JSON start marker is not found.
		return "", fmt.Errorf("cannot find opening JSON marker in %s", input)
	}

	// Start after the marker.
	startIdx += len(StartJsonMarker)

	// Find the closing marker after the start.
	endIdx := strings.Index(input[startIdx:], EndJsonMarker)
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

func normalizeActionPattern(pattern string) string {
	// Remove variable placeholders
	vars := regexp.MustCompile(`\{[^}]+\}`)
	return vars.ReplaceAllString(pattern, "")
}
