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
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/lithammer/shortuuid/v4"
)

const (
	DefaultCompensationTTL  = 24 * time.Hour
	MaxCompensationAttempts = 10
	CompensationBackoffMax  = 1 * time.Minute
)

func NewCompensationWorker(orchestrationID string, logManager *LogManager, candidates []CompensationCandidate, cancel context.CancelFunc) LogWorker {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = CompensationBackoffMax
	expBackoff.Multiplier = 2.0
	expBackoff.RandomizationFactor = 0.1
	expBackoff.MaxElapsedTime = 0 // No max elapsed time - we'll control via attempts

	return &CompensationWorker{
		OrchestrationID: orchestrationID,
		LogManager:      logManager,
		Candidates:      candidates,
		backOff:         expBackoff,
		attemptCounts:   make(map[string]int),
		cancel:          cancel,
	}
}

func (w *CompensationWorker) Start(ctx context.Context, orchestrationID string) {
	for _, candidate := range w.Candidates {
		if err := w.processCandidate(ctx, candidate); err != nil {
			w.LogManager.Logger.Error().
				Err(err).
				Str("orchestrationID", orchestrationID).
				Interface("candidate", candidate).
				Msg("Compensation worker failed to process candidate")

			status := CompensationFailed
			logType := CompensationFailureLogType

			if strings.Contains(err.Error(), "expired") {
				status = CompensationExpired
				logType = CompensationExpiredLogType
			}

			// Log the compensation failure
			failureResult := CompensationResult{
				Status: status,
				Error:  err.Error(),
			}
			if err := w.LogManager.AppendCompensationFailure(
				orchestrationID,
				candidate.TaskID,
				logType,
				failureResult,
				w.attemptCounts[candidate.TaskID],
			); err != nil {
				w.LogManager.Logger.Error().
					Err(err).
					Msg("Failed to log compensation failure")
			}
		}
	}
}

func (w *CompensationWorker) PollLog(_ context.Context, _ string, _ *Log, _ chan<- LogEntry) {
	// no-op
}

func (w *CompensationWorker) processCandidate(ctx context.Context, candidate CompensationCandidate) error {
	return w.executeCompensation(ctx, candidate)
}

func (w *CompensationWorker) executeCompensation(ctx context.Context, candidate CompensationCandidate) error {
	taskID := candidate.TaskID
	service := candidate.Service
	compData := candidate.Compensation

	operation := func() error {
		w.attemptCounts[taskID]++

		ttl := DefaultCompensationTTL
		if compData.TTLMs > 0 {
			ttl = time.Duration(compData.TTLMs) * time.Millisecond
		}

		expiresAt := time.Now().UTC().Add(ttl)
		if time.Now().UTC().After(expiresAt) {
			err := fmt.Errorf("compensation data expired for task %s", taskID)
			return backoff.Permanent(err)
		}

		serviceHealthy := w.LogManager.controlPlane.WebSocketManager.IsServiceHealthy(service.ID)
		if !serviceHealthy {
			w.LogManager.Logger.Warn().
				Str("serviceID", service.ID).
				Int("attempt", w.attemptCounts[taskID]).
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
				if w.attemptCounts[taskID] >= MaxCompensationAttempts {
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
			Input:           compData.Input,
			OrchestrationID: w.OrchestrationID,
			ProjectID:       service.ProjectID,
			Status:          Processing,
		}

		if err := w.LogManager.AppendCompensationAttempted(
			w.OrchestrationID,
			taskID,
			executionID,
			task.Input,
			w.attemptCounts[taskID],
		); err != nil {
			return err
		}

		// Send task to service - attempt even if unhealthy
		if err := w.LogManager.controlPlane.WebSocketManager.SendTask(service.ID, task); err != nil {
			w.LogManager.Logger.Error().
				Err(err).
				Str("taskID", taskID).
				Int("attempt", w.attemptCounts[taskID]).
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

		logType := CompensationCompleteLogType
		if compResult.Status == CompensationPartial {
			logType = CompensationPartialLogType
		}

		return w.LogManager.AppendCompensationComplete(
			w.OrchestrationID,
			taskID,
			logType,
			&compResult,
			w.attemptCounts[taskID],
		)
	}

	return backoff.RetryNotify(operation, w.backOff,
		func(err error, duration time.Duration) {
			w.LogManager.Logger.Info().
				Str("taskID", taskID).
				Int("attempt", w.attemptCounts[taskID]).
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
