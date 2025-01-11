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
	dependencies DependencyKeySet,
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
	expBackoff.MaxElapsedTime = 5 * time.Minute  // Total time to keep retrying

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

	if !containsAll(w.logState.DependencyState, w.Dependencies) {
		return nil
	}

	processingTs := time.Now().UTC()
	if err := w.LogManager.AppendTaskStatusEvent(orchestrationID, w.TaskID, w.Service.ID, Processing, nil, processingTs, w.consecutiveErrs); err != nil {
		return err
	}
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
		// Check service health and respect MaxServiceDowntime
		if err := w.checkServiceHealth(orchestrationID); err != nil {
			return err // Returns RetryableError or permanent error if timeout exceeded
		}

		var err error
		result, err = w.tryExecute(ctx, orchestrationID)
		if err != nil {
			w.consecutiveErrs++
			if w.stopRetryingTask() {
				return backoff.Permanent(fmt.Errorf("too many consecutive failures: %w", err))
			}
			if err := w.LogManager.AppendTaskStatusEvent(
				orchestrationID,
				w.TaskID,
				w.Service.ID,
				Failed,
				err,
				time.Now().UTC(),
				w.consecutiveErrs,
			); err != nil {
				w.LogManager.Logger.Error().Err(err).Msg("Failed to append failed status after retry")
			}

			if _, ok := err.(RetryableError); !ok {
				return backoff.Permanent(err)
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
				w.LogManager.Logger.Error().Err(err).Msg("Failed to append processing status after retry")
			}

			return err
			//if _, ok := err.(RetryableError); ok {
			//	return err
			//}
			//return backoff.Permanent(err)
		}

		// Reset consecutive errors on success
		w.consecutiveErrs = 0
		return nil
	}

	err := backoff.RetryNotify(operation, w.backOff, func(err error, duration time.Duration) {
		if retryErr, ok := err.(RetryableError); ok {
			w.LogManager.Logger.Info().
				Str("OrchestrationID", orchestrationID).
				Str("TaskID", w.TaskID).
				Err(retryErr.Err).
				Dur("RetryAfter", duration).
				Msg("Retrying task due to retryable error")
		}
	})

	if err != nil {
		return nil, err
	}

	return result, nil
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
	return w.consecutiveErrs > maxRetries
}

func (w *TaskWorker) checkServiceHealth(orchestrationID string) error {
	isServiceHealthy := w.isServiceHealthy()
	w.LogManager.Logger.
		Trace().
		Str("TaskID", w.TaskID).
		Bool("isServiceHealthy", isServiceHealthy).
		Msg("checkServiceHealth: isServiceHealthy")

	if isServiceHealthy && !w.pauseStart.IsZero() {
		// Reset pause tracking when service is healthy
		if err := w.LogManager.AppendTaskStatusEvent(
			orchestrationID,
			w.TaskID,
			w.Service.ID,
			Processing,
			nil,
			time.Now().UTC(),
			w.consecutiveErrs,
		); err != nil {
			w.LogManager.Logger.Error().Err(err).Msg("Failed to append processing status after paused status")
		}

		w.pauseStart = time.Time{}
		return nil
	}

	// Start tracking pause time if not already tracking
	if w.pauseStart.IsZero() {
		w.LogManager.Logger.
			Trace().
			Str("TaskID", w.TaskID).
			Bool("isServiceHealthy", isServiceHealthy).
			Msg("checkServiceHealth: START PAUSE")

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
			w.LogManager.Logger.Error().Err(err).Msg("Failed to append paused status")
		}
	}

	// Check if we've exceeded MaxServiceDowntime
	if time.Since(w.pauseStart) >= w.HealthCheckGracePeriod {
		w.LogManager.Logger.
			Trace().
			Str("TaskID", w.TaskID).
			Bool("isServiceHealthy", isServiceHealthy).
			Msg("checkServiceHealth: EXCEEDED MaxServiceDowntime - TERMINATE TASK")

		return backoff.Permanent(fmt.Errorf("service %s remained unhealthy while exceeding maximum duration of %v",
			w.Service.ID, w.HealthCheckGracePeriod))
	}

	w.LogManager.Logger.
		Trace().
		Str("TaskID", w.TaskID).
		Bool("isServiceHealthy", isServiceHealthy).
		Msg("checkServiceHealth: WITHIN MaxServiceDowntime - TRY Again using RetryableError")

	return RetryableError{Err: fmt.Errorf("service %s is not healthy", w.Service.ID)}
}

func (w *TaskWorker) tryExecute(ctx context.Context, orchestrationID string) (json.RawMessage, error) {
	idempotencyKey := w.generateIdempotencyKey(orchestrationID)
	executionID := fmt.Sprintf("e_%s", shortuuid.New())

	// Initialize or get existing execution
	result, isNewExecution, err := w.Service.IdempotencyStore.InitializeExecution(idempotencyKey, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize execution: %w", err)
	}

	// If there's an existing execution
	if !isNewExecution {
		switch {
		case result.State == ExecutionCompleted:
			w.LogManager.Logger.
				Trace().
				Str("orchestrationID", orchestrationID).
				Str("TaskID", w.TaskID).
				Bool("isNewExecution", isNewExecution).
				Str("State", "ExecutionCompleted").
				Msg("Checked for existing task worker execution")
			return result.Result, nil
		case result.State == ExecutionFailed:
			w.LogManager.Logger.
				Trace().
				Str("orchestrationID", orchestrationID).
				Str("TaskID", w.TaskID).
				Bool("isNewExecution", isNewExecution).
				Str("State", "ExecutionFailed").
				Msg("Checked for existing task worker execution")
			return nil, RetryableError{Err: result.Error}
		case result.State == ExecutionPaused:
			// Allow retrying if execution was paused
			w.LogManager.Logger.
				Trace().
				Str("orchestrationID", orchestrationID).
				Str("TaskID", w.TaskID).
				Bool("isNewExecution", isNewExecution).
				Str("State", "ExecutionPaused").
				Msg("Checked for existing task worker execution")

			if _, resumed := w.Service.IdempotencyStore.ResumeExecution(idempotencyKey); !resumed {
				w.LogManager.Logger.
					Trace().
					Str("orchestrationID", orchestrationID).
					Str("TaskID", w.TaskID).
					Bool("isNewExecution", isNewExecution).
					Str("State", "ExecutionPaused").
					Msg("Force resume using RetryableError when checking for existing task worker execution")

				return nil, RetryableError{Err: fmt.Errorf("execution is paused")}
			}
		case result.State == ExecutionInProgress:
			w.LogManager.Logger.
				Trace().
				Str("orchestrationID", orchestrationID).
				Str("TaskID", w.TaskID).
				Bool("isNewExecution", isNewExecution).
				Str("State", "ExecutionInProgress").
				Msg("Checked for existing task worker execution - DO NOTHING")
			// do nothing
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
	input, err := mergeValueMapsToJson(w.logState.DependencyState)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

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

	w.LogManager.Logger.
		Trace().
		Str("orchestrationID", orchestrationID).
		Str("TaskID", w.TaskID).
		Str("ExecutionID", executionID).
		Str("IdempotencyKey", string(key)).
		Str("ServiceName", w.Service.Name).
		Msg("Executing task request - about to send task")

	if err := w.LogManager.controlPlane.WebSocketManager.SendTask(w.Service.ID, task); err != nil {
		w.LogManager.Logger.
			Trace().
			Str("orchestrationID", orchestrationID).
			Str("TaskID", w.TaskID).
			Str("ExecutionID", executionID).
			Str("IdempotencyKey", string(key)).
			Str("ServiceName", w.Service.Name).
			Err(err).
			Msg("Failed to send task request to service - trying again using RetryableError")

		// Pause execution before returning error
		w.Service.IdempotencyStore.PauseExecution(key)
		return nil, RetryableError{Err: fmt.Errorf("failed to send task: %w", err)}
	}

	return w.waitForResult(ctx, key, executionID)
}

func (w *TaskWorker) waitForResult(ctx context.Context, key IdempotencyKey, executionID string) (json.RawMessage, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	maxWait := time.After(w.Timeout)

	for {
		select {
		case <-ctx.Done():
			w.Service.IdempotencyStore.PauseExecution(key)
			return nil, ctx.Err()

		case <-maxWait:
			w.LogManager.Logger.
				Trace().
				Str("TaskID", w.TaskID).
				Str("IdempotencyKey", string(key)).
				Str("IdempotencyOperation", "PauseExecution(key)").
				Msg("Wait for Result: Max Wait has been reached - Try again with RetryableError")

			w.Service.IdempotencyStore.PauseExecution(key)
			return nil, RetryableError{Err: fmt.Errorf("task execution timed out waiting for result")}

		case <-ticker.C:
			w.LogManager.Logger.
				Trace().
				Str("TaskID", w.TaskID).
				Str("IdempotencyKey", string(key)).
				Str("IdempotencyOperation", "GetExecutionWithResult(key)").
				Msg("Wait for Result: What is the state of the current result")

			result, exists := w.Service.IdempotencyStore.GetExecutionWithResult(key)
			if !exists {
				w.LogManager.Logger.
					Trace().
					Str("TaskID", w.TaskID).
					Str("IdempotencyKey", string(key)).
					Str("IdempotencyOperation", "GetExecutionWithResult(key)").
					Msg("Wait for Result: NO RESULT FOUND - Try again with RetryableError")

				return nil, RetryableError{Err: fmt.Errorf("execution not found")}
			}

			switch {
			case result.State == ExecutionCompleted:
				w.LogManager.Logger.
					Trace().
					Str("TaskID", w.TaskID).
					Str("IdempotencyKey", string(key)).
					Str("State", "ExecutionCompleted").
					Msg("Wait for Result: Done")

				return result.Result, nil
			case result.State == ExecutionFailed && result.ExecutionID == executionID:
				w.LogManager.Logger.
					Trace().
					Str("TaskID", w.TaskID).
					Str("IdempotencyKey", string(key)).
					Str("ExecutionID", executionID).
					Str("State", "ExecutionFailed").
					Msg("Wait for Result: Failed for executionID - Try again with RetryableError")
				return nil, RetryableError{Err: result.Error}
			case result.State == ExecutionPaused:
				w.LogManager.Logger.
					Trace().
					Str("TaskID", w.TaskID).
					Str("IdempotencyKey", string(key)).
					Str("ExecutionID", executionID).
					Str("State", "ExecutionPaused").
					Msg("Wait for Result: Try again with RetryableError")

				return nil, RetryableError{Err: fmt.Errorf("execution is paused")}
			case result.State == ExecutionInProgress:
				w.LogManager.Logger.
					Trace().
					Str("TaskID", w.TaskID).
					Str("IdempotencyKey", string(key)).
					Str("ExecutionID", executionID).
					Str("State", "ExecutionInProgress").
					Msg("Wait for Result: DO NOTHING")

				// do nothing
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
					Str("TaskID", w.TaskID).
					Msg("Stopping lease renewal by pausing execution on IdempotencyStore due to unhealthy service")

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

func mergeValueMapsToJson(src map[string]json.RawMessage) (json.RawMessage, error) {
	out := make(map[string]any)
	for _, input := range src {
		if err := json.Unmarshal(input, &out); err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

func containsAll(s map[string]json.RawMessage, e map[string]struct{}) bool {
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
