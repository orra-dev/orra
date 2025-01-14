/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/lithammer/shortuuid/v4"
)

var (
	maxRetries = 3
)

type RetryableError struct {
	Err error
}

func (e RetryableError) Error() string {
	return fmt.Sprintf("%v", e.Err)
}

func NewTaskWorker(
	service *ServiceInfo,
	taskID string,
	dependencies TaskDependenciesWithKeys,
	timeout time.Duration,
	healthCheckGracePeriod time.Duration,
	logManager *LogManager,
) LogWorker {
	expBackoff := backoff.NewExponentialBackOff()

	// Configure backoff parameters
	expBackoff.InitialInterval = 2 * time.Second // Start with 2 seconds delay
	expBackoff.MaxInterval = 30 * time.Second    // Cap maximum delay at 30 seconds
	expBackoff.Multiplier = 2.0                  // Double the delay each time
	expBackoff.RandomizationFactor = 0.1         // Add some jitter
	expBackoff.MaxElapsedTime = 0                // Use consecutiveErrs, timeout, healthCheckGracePeriod for permanent backoff

	// Reset timer to apply our changes
	expBackoff.Reset()

	return &TaskWorker{
		Service:                service,
		TaskID:                 taskID,
		Dependencies:           dependencies,
		Timeout:                timeout,
		HealthCheckGracePeriod: healthCheckGracePeriod,
		LogManager:             logManager,
		logState: &LogState{
			LastOffset:      0,
			Processed:       make(map[string]bool),
			DependencyState: make(map[string]json.RawMessage),
		},
		pauseStart: time.Time{},
		backOff:    expBackoff,
	}
}

func (w *TaskWorker) Start(ctx context.Context, orchestrationID string) {
	logStream := w.LogManager.GetLog(orchestrationID)
	if logStream == nil {
		w.LogManager.Logger.Debug().Str("orchestrationID", orchestrationID).Msg("Log stream not found for orchestration")
		return
	}

	// Channel to receive new log entries
	entriesChan := make(chan LogEntry, 100)

	// Start a goroutine for continuous polling
	go w.PollLog(ctx, orchestrationID, logStream, entriesChan)

	// Process entries as they come in
	for {
		select {
		case entry := <-entriesChan:
			if err := w.processEntry(ctx, entry, orchestrationID); err != nil {
				w.LogManager.Logger.
					Error().
					Err(err).
					Str("orchestrationID", orchestrationID).
					Interface("entry", entry).
					Msgf("Task worker %s failed to process entry for orchestration", w.TaskID)
				return
			}
		case <-ctx.Done():
			w.LogManager.Logger.Info().Msgf("TaskWorker for task %s in orchestration %s is stopping", w.TaskID, orchestrationID)
			return
		}
	}
}

func (w *TaskWorker) PollLog(ctx context.Context, _ string, logStream *Log, entriesChan chan<- LogEntry) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var processableEntries []LogEntry

			entries := logStream.ReadFrom(w.logState.LastOffset)
			for _, entry := range entries {
				if !w.shouldProcess(entry) {
					continue
				}

				processableEntries = append(processableEntries, entry)
				select {
				case entriesChan <- entry:
					w.logState.LastOffset = entry.Offset() + 1
				case <-ctx.Done():
					return
				}
			}

			//w.LogManager.Logger.Debug().
			//	Interface("entries", processableEntries).
			//	Msgf("polling entries for task %s - orchestration %s", w.TaskID, orchestrationID)
		case <-ctx.Done():
			return
		}
	}
}

func (w *TaskWorker) shouldProcess(entry LogEntry) bool {
	_, isDependency := w.Dependencies[entry.ID()]
	processed := w.logState.Processed[entry.ID()]
	return entry.Type() == "task_output" && isDependency && !processed
}

func (w *TaskWorker) processEntry(ctx context.Context, entry LogEntry, orchestrationID string) error {
	// Store the entry's output in our dependency state
	w.logState.DependencyState[entry.ID()] = entry.Value()

	if !taskDependenciesMet(w.logState.DependencyState, w.Dependencies) {
		return nil
	}

	processingTs := time.Now().UTC()
	if err := w.LogManager.MarkTask(orchestrationID, w.TaskID, Processing, processingTs); err != nil {
		return err
	}

	// Execute our task
	taskOutput, err := w.executeTaskWithRetry(ctx, orchestrationID)
	if err != nil {
		w.LogManager.Logger.Error().Err(err).Msgf("Cannot execute task %s for orchestration %s", w.TaskID, orchestrationID)
		failedTs := time.Now().UTC()
		if err := w.LogManager.AppendTaskStatusEvent(orchestrationID, w.TaskID, w.Service.ID, Failed, err, failedTs, w.consecutiveErrs); err != nil {
			return err
		}
		if err := w.LogManager.MarkTask(orchestrationID, w.TaskID, Failed, failedTs); err != nil {
			return err
		}
		return w.LogManager.AppendTaskFailureToLog(orchestrationID, w.TaskID, w.Service.ID, err.Error(), w.consecutiveErrs, false)
	}

	if err := w.processTaskResult(orchestrationID, taskOutput); err != nil {
		w.LogManager.Logger.Error().Err(err).Msgf("Cannot process task %s result for orchestration %s", w.TaskID, orchestrationID)
		return w.LogManager.AppendTaskFailureToLog(
			orchestrationID,
			w.TaskID,
			w.Service.ID,
			err.Error(),
			w.consecutiveErrs,
			false,
		)
	}

	// Mark this entry as processed
	w.logState.Processed[entry.ID()] = true

	completedTs := time.Now().UTC()
	if err = w.LogManager.AppendTaskStatusEvent(orchestrationID, w.TaskID, w.Service.ID, Completed, nil, completedTs, w.consecutiveErrs); err != nil {
		return err
	}

	if err := w.LogManager.MarkTaskCompleted(orchestrationID, entry.ID(), completedTs); err != nil {
		w.LogManager.Logger.Error().Err(err).Msgf("Cannot mark task %s completed for orchestration %s", w.TaskID, orchestrationID)
		return w.LogManager.AppendTaskFailureToLog(orchestrationID, w.TaskID, w.Service.ID, err.Error(), w.consecutiveErrs, false)
	}

	return nil
}

func (w *TaskWorker) executeTaskWithRetry(ctx context.Context, orchestrationID string) (json.RawMessage, error) {
	var result json.RawMessage
	w.consecutiveErrs = 0

	operation := func() error {
		logger := w.LogManager.Logger.With().
			Str("Operation", "executeTaskWithRetry").
			Str("OrchestrationID", orchestrationID).
			Str("TaskID", w.TaskID).
			Str("Service", w.Service.Name).
			Int("ConsecutiveErrs", w.consecutiveErrs).
			Logger()

		// Check service health and respect MaxServiceDowntime
		if err := w.checkServiceHealth(orchestrationID); err != nil {
			return err // Returns RetryableError or permanent error if timeout exceeded
		}

		var err error
		result, err = w.tryExecute(ctx, orchestrationID)
		if err != nil {
			if w.triggerPauseExecution(err) {
				return err
			}

			w.consecutiveErrs++
			if w.stopRetryingTask() {
				logger.Trace().Err(err).Msg("Stop retrying task - too many consecutive failures")
				return backoff.Permanent(fmt.Errorf("too many consecutive failures: %w", err))
			}

			logger.Trace().Err(err).Msg("Retrying failed task")

			if err := w.LogManager.AppendTaskStatusEvent(
				orchestrationID,
				w.TaskID,
				w.Service.ID,
				Failed,
				err,
				time.Now().UTC(),
				w.consecutiveErrs,
			); err != nil {
				logger.Error().Err(err).Msg("Failed to append failed status after retry")
			}

			return err
		}

		// Reset consecutive errors on success
		w.consecutiveErrs = 0
		return nil
	}

	err := backoff.RetryNotify(operation, w.backOff, func(err error, duration time.Duration) {
		if retryErr, ok := err.(RetryableError); ok {
			w.LogManager.Logger.Info().
				Err(retryErr.Err).
				Str("Operation", "backoff.RetryNotify: executeTaskWithRetry").
				Str("OrchestrationID", orchestrationID).
				Str("TaskID", w.TaskID).
				Str("Service", w.Service.Name).
				Dur("RetryAfter", duration).
				Int("ConsecutiveErrs", w.consecutiveErrs).
				Msg("Retrying task due to retryable error")
		}
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (w *TaskWorker) triggerPauseExecution(err error) bool {
	if _, ok := err.(RetryableError); !ok {
		return false
	}

	if err.Error() != PauseExecutionCode {
		return false
	}

	return true
}

func (w *TaskWorker) processTaskResult(orchestrationID string, output json.RawMessage) error {
	var resultPayload TaskResultPayload
	if err := json.Unmarshal(output, &resultPayload); err != nil {
		return fmt.Errorf(
			"failed to unmarshal task [%s] result for orchestration [%s]: %v",
			w.TaskID,
			orchestrationID,
			err,
		)
	}

	// Store the task result first
	w.LogManager.AppendToLog(
		orchestrationID,
		"task_output",
		w.TaskID,
		resultPayload.Task,
		w.Service.ID,
		w.consecutiveErrs,
	)

	if !w.Service.Revertible {
		return nil
	}

	if err := w.LogManager.AppendCompensationDataStored(
		orchestrationID,
		w.TaskID,
		w.Service.ID,
		resultPayload.Compensation,
	); err != nil {
		return fmt.Errorf(
			"failed to store compensation data for task [%s] result for orchestration [%s]: %v",
			w.TaskID,
			orchestrationID,
			err,
		)
	}

	return nil
}

func (w *TaskWorker) stopRetryingTask() bool {
	return w.consecutiveErrs >= maxRetries
}

func (w *TaskWorker) checkServiceHealth(orchestrationID string) error {
	isServiceHealthy := w.isServiceHealthy()
	logger := w.LogManager.Logger.
		With().
		Str("Operation", "checkServiceHealth").
		Str("OrchestrationID", orchestrationID).
		Str("TaskID", w.TaskID).
		Str("Service", w.Service.Name).
		Bool("isServiceHealthy", isServiceHealthy).Logger()

	if isServiceHealthy {
		if !w.pauseStart.IsZero() {
			logger.Trace().Msg("reset service health")
		}
		w.pauseStart = time.Time{}
		return nil
	}

	// Start tracking pause time if not already tracking
	if w.pauseStart.IsZero() {
		logger.Trace().Msg("start pausing task")

		w.pauseStart = time.Now().UTC()
		if err := w.LogManager.AppendTaskStatusEvent(
			orchestrationID,
			w.TaskID,
			w.Service.ID,
			Paused,
			fmt.Errorf("service %s is not healthy", w.Service.ID),
			w.pauseStart,
			w.consecutiveErrs,
		); err != nil {
			logger.Error().Err(err).Msg("Failed to append paused status")
		}
	}

	// Check if we've exceeded MaxServiceDowntime
	if time.Since(w.pauseStart) >= w.HealthCheckGracePeriod {
		logger.Trace().Msg("EXCEEDED MaxServiceDowntime - TERMINATE TASK")

		return backoff.Permanent(fmt.Errorf("service %s remained unhealthy while exceeding maximum duration of %v",
			w.Service.ID, w.HealthCheckGracePeriod))
	}

	logger.Trace().Msg("KEEP PAUSING TASK")
	//w.backOff.Reset() --> possibly useful, will check after the base algo is working again.
	return RetryableError{Err: fmt.Errorf("service %s is not healthy - pause %s", w.Service.ID, w.TaskID)}
}

func (w *TaskWorker) tryExecute(ctx context.Context, orchestrationID string) (json.RawMessage, error) {
	idempotencyKey := w.generateIdempotencyKey(orchestrationID)
	executionID := fmt.Sprintf("e_%s", shortuuid.New())

	// Initialize or get existing execution
	result, isNewExecution, err := w.Service.IdempotencyStore.InitializeOrGetExecution(idempotencyKey, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize execution: %w", err)
	}

	logger := w.LogManager.Logger.With().
		Str("Operation", "Post IdempotencyStore.InitializeOrGetExecution").
		Str("orchestrationID", orchestrationID).
		Str("TaskID", w.TaskID).
		Str("ServiceName", w.Service.Name).
		Bool("isNewExecution", isNewExecution).
		Logger()

	if !isNewExecution {
		switch {
		case result.State == ExecutionCompleted:
			logger.Trace().Str("State", "ExecutionCompleted").Msg("OLD EXECUTION")
			return result.Result, nil
		case result.State == ExecutionFailed:
			w.Service.IdempotencyStore.ResetFailedExecution(idempotencyKey)
			logger.Trace().Str("State", "ExecutionFailed").Msg("OLD EXECUTION")
		case result.State == ExecutionPaused:
			logger.Trace().Str("State", "ExecutionPaused").Msg("OLD EXECUTION")
		case result.State == ExecutionInProgress:
			logger.Trace().Str("State", "ExecutionInProgress").Msg("DO NOTHING")
		}

	} else {

		switch {
		case result.State == ExecutionCompleted:
			logger.Trace().Str("State", "ExecutionCompleted").Msg("NEW EXECUTION")
		case result.State == ExecutionFailed:
			logger.Trace().Str("State", "ExecutionFailed").Msg("NEW EXECUTION")
		case result.State == ExecutionPaused:
			logger.Trace().Str("State", "ExecutionPaused").Msg("NEW EXECUTION")
		case result.State == ExecutionInProgress:
			logger.Trace().Str("State", "ExecutionInProgress").Msg("NEW EXECUTION")
		}
	}

	// Start lease renewal
	renewalCtx, cancelRenewal := context.WithCancel(ctx)
	defer cancelRenewal()
	go w.renewLeaseWithHealthCheck(renewalCtx, idempotencyKey, executionID)

	// Execute the actual task
	return w.executeTask(ctx, orchestrationID, idempotencyKey, executionID)
}

func (w *TaskWorker) executeTask(ctx context.Context, orchestrationID string, key IdempotencyKey, executionID string) (json.RawMessage, error) {
	input, err := mergeValueMapsToJson(w.logState.DependencyState, w.Dependencies)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	logger := w.LogManager.Logger.
		With().
		Str("Operation", "executeTask").
		Str("orchestrationID", orchestrationID).
		Str("TaskID", w.TaskID).
		Str("ServiceName", w.Service.Name).
		Str("ExecutionID", executionID).
		Str("IdempotencyKey", string(key)).
		Int("ConsecutiveErrors", w.consecutiveErrs).Logger()

	var tempInput map[string]interface{}
	_ = json.Unmarshal(input, &tempInput)
	logger.Trace().
		Interface("Input Dependencies", w.Dependencies).
		Interface("Input", tempInput).
		Msg("Task input")

	task := &Task{
		Type:            "task_request",
		ID:              w.TaskID,
		ExecutionID:     executionID,
		IdempotencyKey:  key,
		ServiceID:       w.Service.ID,
		Input:           input,
		OrchestrationID: orchestrationID,
		ProjectID:       w.Service.ProjectID,
		Status:          Processing,
	}

	logger.Trace().Msg("Executing task request - about to send task")

	if err := w.LogManager.controlPlane.WebSocketManager.SendTask(w.Service.ID, task); err != nil {
		logger.Trace().Err(err).Msg("Failed to send task request to service - trying again using RetryableError")

		// Pause execution before returning error
		w.Service.IdempotencyStore.PauseExecution(key)
		return nil, RetryableError{Err: fmt.Errorf("failed to send task: %w", err)}
	}

	if err := w.LogManager.AppendTaskStatusEvent(
		orchestrationID,
		w.TaskID,
		w.Service.ID,
		Processing,
		nil,
		time.Now().UTC(),
		w.consecutiveErrs,
	); err != nil {
		logger.Error().Err(err).Msg("Failed to append processing status after paused status")
	}

	return w.waitForResult(ctx, key, executionID)
}

func (w *TaskWorker) waitForResult(ctx context.Context, key IdempotencyKey, executionID string) (json.RawMessage, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	maxWait := time.After(w.Timeout)

	logger := w.LogManager.Logger.With().
		Str("Operation", "waitForResult").
		Str("TaskID", w.TaskID).
		Str("IdempotencyKey", string(key)).
		Str("ExecutionID", executionID).
		Str("Service", w.Service.Name).
		Int("consecutiveErrs", w.consecutiveErrs).
		Logger()

	for {
		select {
		case <-ctx.Done():
			logger.Trace().Msg("Task request cancelled - ctx.Done()")
			w.Service.IdempotencyStore.PauseExecution(key)
			return nil, ctx.Err()

		case <-maxWait:
			logger.Trace().Msg("Task request has reached Max Wait - RETRY")
			w.Service.IdempotencyStore.PauseExecution(key)
			return nil, RetryableError{Err: fmt.Errorf("task execution timed out waiting for result")}

		case <-ticker.C:
			result, exists := w.Service.IdempotencyStore.GetExecutionWithResult(key)
			if !exists {
				logger.Trace().
					Str("SubOperation", "GetExecutionWithResult(key)").
					Msg("NO RESULT FOUND - CONTINUE")
				continue
			}

			switch {
			case result.State == ExecutionCompleted:
				logger.Trace().Str("State", "ExecutionCompleted").Msg("Completed with result")
				return result.Result, nil
			case result.State == ExecutionFailed:
				if err, b := result.GetFailure(w.consecutiveErrs); b {
					logger.Trace().Str("State", "ExecutionFailed").Msg("Failed - RETRY")
					return nil, RetryableError{Err: err}
				}
				logger.Trace().Str("State", "ExecutionFailed").Msg("Failed but no failure entry- DO NOTHING")
			case result.State == ExecutionPaused:
				logger.Trace().Str("State", "ExecutionPaused").Msg("PAUSED - Trigger Pause")
				return nil, RetryableError{Err: fmt.Errorf(PauseExecutionCode)}
			case result.State == ExecutionInProgress:
				logger.Trace().Str("State", "ExecutionInProgress").Msg("DO NOTHING")
			}
		}
	}
}

func (w *TaskWorker) renewLeaseWithHealthCheck(ctx context.Context, key IdempotencyKey, executionID string) {
	ticker := time.NewTicker(defaultLeaseDuration / 2) // Renew at half the lease duration
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, stop renewal
			return

		case <-ticker.C:
			// Check service health first
			if !w.isServiceHealthy() {
				w.LogManager.Logger.
					Trace().
					Str("Operation", "renewLeaseWithHealthCheck").
					Str("TaskID", w.TaskID).
					Str("Service", w.Service.Name).
					Str("IdempotencyKey", string(key)).
					Str("ExecutionID", executionID).
					Int("consecutiveErrs", w.consecutiveErrs).
					Msg("PauseExecution - Stop lease renewal due to unhealthy service")
				w.Service.IdempotencyStore.PauseExecution(key)
				return
			}

			// Try to renew lease
			renewed := w.Service.IdempotencyStore.RenewLease(key, executionID)
			if !renewed {
				// If we couldn't renew, the execution might be paused or taken over
				w.LogManager.Logger.Trace().
					Str("TaskID", w.TaskID).
					Msg("Lease renewal failed, stopping renewal routine")
				return
			}
		}
	}
}

func mergeValueMapsToJson(src map[string]json.RawMessage, dependencies TaskDependenciesWithKeys) (json.RawMessage, error) {
	out := make(map[string]any)
	for depID, input := range src {
		temp := make(map[string]any)
		if err := json.Unmarshal(input, &temp); err != nil {
			return nil, err
		}

		for _, k := range dependencies[depID] {
			if _, ok := temp[k]; !ok {
				continue
			}
			out[k] = temp[k]
		}
	}
	return json.Marshal(out)
}

func taskDependenciesMet(s map[string]json.RawMessage, e TaskDependenciesWithKeys) bool {
	for srcId := range e {
		if _, hasOutput := s[srcId]; !hasOutput {
			return false
		}
	}
	return true
}

func (w *TaskWorker) isServiceHealthy() bool {
	return w.LogManager.controlPlane.WebSocketManager.IsServiceHealthy(w.Service.ID)
}

func (w *TaskWorker) generateIdempotencyKey(orchestrationID string) IdempotencyKey {
	h := sha256.New()
	h.Write([]byte(orchestrationID))
	h.Write([]byte(w.TaskID))

	inputs := w.sortedInputs()
	for _, input := range inputs {
		h.Write([]byte(input))
	}

	return IdempotencyKey(fmt.Sprintf("%x", h.Sum(nil)))
}

func (w *TaskWorker) sortedInputs() []string {
	var inputs []string
	for k, v := range w.logState.DependencyState {
		inputs = append(inputs, fmt.Sprintf("%s:%s", k, string(v)))
	}
	sort.Strings(inputs)
	return inputs
}
