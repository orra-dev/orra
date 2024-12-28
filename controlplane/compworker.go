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
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/lithammer/shortuuid/v4"
)

const (
	DefaultCompensationTTL  = 24 * time.Hour
	MaxCompensationAttempts = 10
	CompensationBackoffMax  = 1 * time.Minute
)

func NewCompensationWorker(orchestrationID string, logManager *LogManager, taskIDs []string, cancel context.CancelFunc) LogWorker {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = CompensationBackoffMax
	expBackoff.Multiplier = 2.0
	expBackoff.RandomizationFactor = 0.1
	expBackoff.MaxElapsedTime = 0 // No max elapsed time - we'll control via attempts

	dependencies := make(DependencyKeySet)
	for _, taskID := range taskIDs {
		dependencies[taskID] = struct{}{}
	}

	logManager.Logger.
		Debug().
		Interface("dependencies", dependencies).
		Str("orchestrationID", orchestrationID).
		Msg("Attempting to compensate dependencies")

	return &CompensationWorker{
		OrchestrationID: orchestrationID,
		LogManager:      logManager,
		logState: &LogState{
			LastOffset:      0,
			Processed:       make(map[string]bool),
			DependencyState: make(map[string]json.RawMessage),
		},
		Dependencies:  dependencies,
		backOff:       expBackoff,
		attemptCounts: make(map[string]int),
		cancel:        cancel,
	}
}

func (w *CompensationWorker) Start(ctx context.Context, orchestrationID string) {
	logStream := w.LogManager.GetLog(orchestrationID)
	if logStream == nil {
		w.LogManager.Logger.Error().
			Str("orchestrationID", orchestrationID).
			Msg("Log stream not found for compensation")
		return
	}

	// Channel to receive new log entries
	entriesChan := make(chan LogEntry, 100)

	// Start polling log
	go w.PollLog(ctx, orchestrationID, logStream, entriesChan)

	// Process entries as they come in
	for {
		select {
		case entry := <-entriesChan:
			if err := w.processEntry(ctx, entry); err != nil {
				w.LogManager.Logger.Error().
					Err(err).
					Str("orchestrationID", orchestrationID).
					Interface("entry", entry).
					Msg("Compensation worker failed to process entry")

				// Log the compensation failure
				failureResult := CompensationResult{
					Status: CompensationFailed,
					Error:  err.Error(),
				}
				if err := w.LogManager.AppendCompensationFailure(
					orchestrationID,
					entry.ID(),
					failureResult,
					w.attemptCounts[entry.ID()],
				); err != nil {
					w.LogManager.Logger.Error().
						Err(err).
						Msg("Failed to log compensation failure")
				}
			}

			if w.hasCompensatedAllDependencies() {
				w.LogManager.Logger.Info().
					Str("orchestrationID", w.OrchestrationID).
					Int("totalTasks", len(w.Dependencies)).
					Msg("All compensations complete, worker stopping")
				w.cancel() // Self cleanup
			}

		case <-ctx.Done():
			w.LogManager.Logger.Info().
				Str("orchestrationID", orchestrationID).
				Msg("Compensation worker stopping")
			return
		}
	}
}

func (w *CompensationWorker) hasCompensatedAllDependencies() bool {
	for taskID := range w.Dependencies {
		if !w.logState.Processed[taskID] {
			return false
		}
	}
	return true
}

func (w *CompensationWorker) PollLog(ctx context.Context, orchestrationID string, logStream *Log, entriesChan chan<- LogEntry) {
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

			w.LogManager.Logger.Trace().
				Interface("entries", processableEntries).
				Msgf("polling entries for compensating orchestration %s", orchestrationID)

		case <-ctx.Done():
			return
		}
	}
}

func (w *CompensationWorker) shouldProcess(entry LogEntry) bool {
	_, isDependency := w.Dependencies[entry.ID()]
	return entry.Type() == "task_output" && isDependency && !w.logState.Processed[entry.ID()]
}

func (w *CompensationWorker) processEntry(ctx context.Context, entry LogEntry) error {
	w.logState.DependencyState[entry.ID()] = entry.Value()

	// Get compensation data for the task
	compensationData, err := w.getCompensationData(entry.ID())
	if err != nil {
		return fmt.Errorf("failed to get compensation data: %w", err)
	}

	if compensationData == nil {
		w.LogManager.Logger.Debug().
			Str("taskID", entry.ID()).
			Msg("No compensation data found for task, marking as processed")
		w.logState.Processed[entry.ID()] = true
		return nil
	}

	// Execute compensation
	if err := w.executeCompensation(ctx, entry, compensationData); err != nil {
		return err
	}

	// Mark as processed
	w.logState.Processed[entry.ID()] = true

	return nil
	//
	//// Check TTL if provided
	//ttl := DefaultCompensationTTL
	//if compensationData.Meta.TTL > 0 {
	//	ttl = compensationData.Meta.TTL
	//}
	//
	//expiresAt := compensationData.Meta.ExpiresAt
	//if expiresAt.IsZero() {
	//	expiresAt = time.Now().Add(ttl)
	//}
	//
	//if time.Now().After(expiresAt) {
	//	return fmt.Errorf("compensation data expired for task %s", entry.ID())
	//}
	//
	//if err := w.executeWithRetry(ctx, entry.ID(), compensationData); err != nil {
	//	return err
	//}
	//
	//processingTs := time.Now().UTC()
	//if err := w.LogManager.MarkCompensationCompleted(w.OrchestrationID, entry.ID(), processingTs); err != nil {
	//	return err
	//}
}

func (w *CompensationWorker) getCompensationData(taskID string) (*CompensationData, error) {
	logStream := w.LogManager.GetLog(w.OrchestrationID)
	if logStream == nil {
		return nil, fmt.Errorf("log stream not found")
	}

	// Read all entries from beginning
	entries := logStream.ReadFrom(0)
	for _, entry := range entries {
		if entry.Type() == CompensationDataStoredLogType && entry.ProducerID() == taskID {
			var compData CompensationData
			if err := json.Unmarshal(entry.Value(), &compData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal compensation data: %w", err)
			}
			return &compData, nil
		}
	}

	return nil, nil // No compensation data is not an error
}

func (w *CompensationWorker) executeCompensation(ctx context.Context, entry LogEntry, data *CompensationData) error {
	taskID := entry.ID()
	serviceID := entry.ProducerID()

	w.attemptCounts[taskID]++
	currentAttempt := w.attemptCounts[taskID]

	operation := func() error {
		// Don't block on health checks, but log status
		service, err := w.LogManager.controlPlane.GetServiceByID(serviceID)
		if err != nil {
			return backoff.Permanent(fmt.Errorf("failed to get service: %w", err))
		}

		ttl := DefaultCompensationTTL
		if data.TTLMs > 0 {
			ttl = time.Duration(data.TTLMs) * time.Millisecond
		}

		expiresAt := time.Now().UTC().Add(ttl)
		if time.Now().UTC().After(expiresAt) {
			err := fmt.Errorf("compensation data expired for task %s", entry.ID())
			return backoff.Permanent(err)
		}

		serviceHealthy := w.LogManager.controlPlane.WebSocketManager.IsServiceHealthy(service.ID)
		if !serviceHealthy {
			w.LogManager.Logger.Warn().
				Str("serviceID", service.ID).
				Int("attempt", currentAttempt).
				Msg("Attempting compensation despite unhealthy service")
		}

		key := w.generateIdempotencyKey(w.OrchestrationID, taskID)
		executionID := fmt.Sprintf("e_comp_%s", shortuuid.New())

		// Initialize or get existing execution
		execution, isNewExecution, err := service.IdempotencyStore.InitializeExecution(key, executionID)
		if err != nil {
			return fmt.Errorf("failed to initialize execution: %w", err)
		}

		// Check existing execution state
		if !isNewExecution {
			switch execution.State {
			case ExecutionCompleted:
				w.LogManager.Logger.Info().
					Str("taskID", taskID).
					Msg("Compensation already completed successfully")
				return nil
			case ExecutionFailed:
				// Continue retrying if within attempts limit
				if currentAttempt >= MaxCompensationAttempts {
					return backoff.Permanent(fmt.Errorf("max compensation attempts reached: %w", execution.Error))
				}
				return execution.Error
			case ExecutionInProgress:
				// do nothing
			case ExecutionPaused:
				// Not relevant here
			}
		}

		// Create compensation task
		task := &Task{
			Type:            "compensation_request",
			ID:              taskID,
			ExecutionID:     executionID,
			IdempotencyKey:  key,
			ServiceID:       service.ID,
			Input:           data.Input,
			OrchestrationID: w.OrchestrationID,
			ProjectID:       service.ProjectID,
			Status:          Processing,
		}

		if err := w.LogManager.AppendCompensationAttempted(
			w.OrchestrationID,
			taskID,
			executionID,
			task.Input,
			currentAttempt,
		); err != nil {
			return err
		}

		// Send task to service - attempt even if unhealthy
		if err := w.LogManager.controlPlane.WebSocketManager.SendTask(service.ID, task); err != nil {
			w.LogManager.Logger.Error().
				Err(err).
				Str("taskID", taskID).
				Int("attempt", currentAttempt).
				Msg("Failed to send compensation task")
			return err
		}

		// Wait for result
		result, err := w.waitForCompensationResult(ctx, service, key)
		if err != nil {
			return err
		}

		var compResult CompensationResult
		if err := json.Unmarshal(result, &compResult); err != nil {
			return fmt.Errorf("invalid compensation result format: %w", err)
		}

		return w.LogManager.AppendCompensationComplete(
			w.OrchestrationID,
			taskID,
			&compResult,
			currentAttempt,
		)
	}

	return backoff.RetryNotify(operation, w.backOff,
		func(err error, duration time.Duration) {
			w.LogManager.Logger.Info().
				Str("taskID", taskID).
				Int("attempt", currentAttempt).
				Err(err).
				Dur("retryAfter", duration).
				Msg("Retrying compensation")
		})
}

func (w *CompensationWorker) generateIdempotencyKey(orchestrationID, taskID string) IdempotencyKey {
	h := sha256.New()
	h.Write([]byte(orchestrationID))
	h.Write([]byte(taskID))
	return IdempotencyKey(fmt.Sprintf("%x", h.Sum(nil)))
}

func (w *CompensationWorker) waitForCompensationResult(ctx context.Context, service *ServiceInfo, key IdempotencyKey) (json.RawMessage, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	maxWait := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-maxWait:
			return nil, fmt.Errorf("timeout waiting for compensation result")

		case <-ticker.C:
			execution, exists := service.IdempotencyStore.GetExecutionWithResult(key)
			if !exists {
				continue
			}

			switch execution.State {
			case ExecutionCompleted:
				return execution.Result, nil
			case ExecutionFailed:
				return nil, execution.Error
			case ExecutionInProgress:
				continue
			case ExecutionPaused:
				// Not relevant here
			}
		}
	}
}
