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

	back "github.com/cenkalti/backoff/v4"
	short "github.com/lithammer/shortuuid/v4"
)

const (
	DefaultCompensationTTL  = 24 * time.Hour
	MaxCompensationAttempts = 10
	CompensationBackoffMax  = 1 * time.Minute
)

func NewCompensationWorker(orchestrationID string, logManager *LogManager, candidates []CompensationCandidate, cancel context.CancelFunc) LogWorker {
	expBackoff := back.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = CompensationBackoffMax
	expBackoff.Multiplier = 2.0
	expBackoff.RandomizationFactor = 0.1 // Add some jitter
	expBackoff.MaxElapsedTime = 0        // No max elapsed time - we'll control via attempts

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
	taskID := candidate.TaskID
	service := candidate.Service

	logger := w.LogManager.Logger.With().
		Str("Operation", "processCandidate - compworker").
		Str("TaskID", taskID).
		Int("Attempt", w.attemptCounts[taskID]).
		Str("Service", service.Name).
		Logger()

	result, err := w.executeCompensationWithRetry(ctx, candidate)
	if err != nil {
		return err
	}

	logType := CompensationCompleteLogType
	var compResult CompensationResult
	if err := json.Unmarshal(result, &compResult); err != nil {
		logger.Warn().Err(err).Msg("Compensation executed successfully but the result is incorrect")
	} else {
		if compResult.Status == CompensationPartial {
			logType = CompensationPartialLogType
		}
	}

	return w.LogManager.AppendCompensationComplete(
		w.OrchestrationID,
		taskID,
		logType,
		&compResult,
		w.attemptCounts[taskID]+1,
	)
}

func (w *CompensationWorker) executeCompensationWithRetry(ctx context.Context, candidate CompensationCandidate) (json.RawMessage, error) {
	var result json.RawMessage
	taskID := candidate.TaskID
	service := candidate.Service
	compData := candidate.Compensation

	operation := func() error {
		logger := w.LogManager.Logger.With().
			Str("Operation", "executeCompensationWithRetry").
			Str("TaskID", taskID).
			Int("Attempt", w.attemptCounts[taskID]).
			Str("Service", service.Name).
			Logger()

		ttl := DefaultCompensationTTL
		if compData.TTLMs > 0 {
			ttl = time.Duration(compData.TTLMs) * time.Millisecond
		}

		expiresAt := time.Now().UTC().Add(ttl)
		if time.Now().UTC().After(expiresAt) {
			logger.Trace().Time("ExpiresAt", expiresAt).Msg("Compensation data expired")
			err := fmt.Errorf("compensation data expired for task %s", taskID)
			return back.Permanent(err)
		}

		var err error
		result, err = w.tryExecuteCompensation(ctx, candidate)
		if err != nil {
			w.attemptCounts[taskID]++
			if w.stopRetrying(taskID) {
				logger.Trace().Err(err).Msg("MaxCompensationAttempts reached")
				return back.Permanent(fmt.Errorf("max compensation attempts (%d) reached: %w", MaxCompensationAttempts, err))
			}

			logger.Trace().Err(err).Msg("Retrying failed compensation")
			return err
		}

		return nil
	}

	err := back.RetryNotify(operation, w.backOff,
		func(err error, duration time.Duration) {
			w.LogManager.Logger.Info().
				Str("Operation", "executeCompensationWithRetry").
				Str("TaskID", taskID).
				Str("Service", service.Name).
				Int("Attempt", w.attemptCounts[taskID]).
				Err(err).
				Dur("retryAfter", duration).
				Msg("Retrying compensation")
		})

	return result, err
}

func (w *CompensationWorker) tryExecuteCompensation(ctx context.Context, candidate CompensationCandidate) (json.RawMessage, error) {
	taskID := candidate.TaskID
	service := candidate.Service
	compData := candidate.Compensation

	logger := w.LogManager.Logger.With().
		Str("Operation", "tryExecuteCompensation").
		Str("TaskID", taskID).
		Int("Attempt", w.attemptCounts[taskID]).
		Str("Service", service.Name).
		Logger()

	serviceHealthy := w.LogManager.planEngine.WebSocketManager.IsServiceHealthy(service.ID)
	if !serviceHealthy {
		logger.Warn().Msg("Attempt compensation despite unhealthy service")
	}

	idempotencyKey := w.generateIdempotencyKey(w.OrchestrationID, taskID)
	executionID := fmt.Sprintf("e_comp_%s", short.New())

	execution, isNewExecution, err := service.IdempotencyStore.InitializeOrGetExecution(idempotencyKey, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize execution: %w", err)
	}

	if !isNewExecution {
		switch {
		case execution.State == ExecutionCompleted:
			logger.Trace().Str("State", "ExecutionCompleted").Msg("OLD EXECUTION")
			return execution.Result, nil
		case execution.State == ExecutionFailed:
			service.IdempotencyStore.ResetFailedExecution(idempotencyKey)
			logger.Trace().Str("State", "ExecutionFailed").Msg("OLD EXECUTION")
		case execution.State == ExecutionInProgress:
			logger.Trace().Str("State", "ExecutionInProgress").Msg("DO NOTHING")
		}

	} else {

		switch {
		case execution.State == ExecutionCompleted:
			logger.Trace().Str("State", "ExecutionCompleted").Msg("NEW EXECUTION")
		case execution.State == ExecutionFailed:
			logger.Trace().Str("State", "ExecutionFailed").Msg("NEW EXECUTION")
		case execution.State == ExecutionPaused:
			logger.Trace().Str("State", "ExecutionPaused").Msg("NEW EXECUTION")
		case execution.State == ExecutionInProgress:
			logger.Trace().Str("State", "ExecutionInProgress").Msg("NEW EXECUTION")
		}
	}

	task := &Task{
		Type:            "compensation_request",
		ID:              taskID,
		ExecutionID:     executionID,
		IdempotencyKey:  idempotencyKey,
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
		w.attemptCounts[taskID]+1,
	); err != nil {
		return nil, err
	}

	if err := w.LogManager.planEngine.WebSocketManager.SendTask(service.ID, task); err != nil {
		logger.Error().Err(err).Msg("Failed to send compensation task")
		return nil, err
	}

	return w.waitForCompensationResult(ctx, service, taskID, idempotencyKey)
}

func (w *CompensationWorker) stopRetrying(taskID string) bool {
	return w.attemptCounts[taskID] >= MaxCompensationAttempts
}

func (w *CompensationWorker) generateIdempotencyKey(orchestrationID, taskID string) IdempotencyKey {
	h := sha256.New()
	h.Write([]byte(orchestrationID))
	h.Write([]byte(taskID))
	h.Write([]byte(CompensationWorkerID))
	return IdempotencyKey(fmt.Sprintf("%x", h.Sum(nil)))
}

func (w *CompensationWorker) waitForCompensationResult(
	ctx context.Context,
	service *ServiceInfo,
	taskID string,
	key IdempotencyKey,
) (json.RawMessage, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	maxWait := time.After(30 * time.Second)

	logger := w.LogManager.Logger.With().
		Str("Operation", "waitForCompensationResult").
		Str("TaskID", taskID).
		Str("Service", service.Name).
		Str("IdempotencyKey", string(key)).
		Int("AttemptCounts", w.attemptCounts[taskID]).
		Logger()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-maxWait:
			logger.Trace().Str("State", "Compensation timed out").Msg("Time out - RETRY")
			err := fmt.Errorf("timed out waiting for compensation result")
			service.IdempotencyStore.UpdateExecutionResult(
				key,
				nil,
				err,
			)
			return nil, err

		case <-ticker.C:
			execution, exists := service.IdempotencyStore.GetExecutionWithResult(key)
			if !exists {
				continue
			}

			switch {
			case execution.State == ExecutionCompleted:
				logger.Trace().Str("State", "Compensation ExecutionCompleted").Msg("Completed with result")
				return execution.Result, nil
			case execution.State == ExecutionFailed:
				if err, b := execution.GetFailure(w.attemptCounts[taskID]); b {
					logger.Trace().Str("State", "Compensation ExecutionFailed").Msg("Failed - RETRY")
					return nil, err
				}
				logger.Trace().Str("State", "Compensation ExecutionFailed").Msg("Failed but no failure entry- DO NOTHING")
			}
		}
	}
}
