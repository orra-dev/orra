/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"sync"
	"time"
)

type IdempotencyKey string

type ExecutionState int

const (
	ExecutionInProgress ExecutionState = iota
	ExecutionPaused
	ExecutionCompleted
	ExecutionFailed
)

const (
	defaultLeaseDuration = 30 * time.Second
	defaultStoreTTL      = 24 * time.Hour
)

func (s ExecutionState) String() string {
	return [...]string{"in_progress", "paused", "completed", "failed"}[s]
}

type IdempotencyStore struct {
	mu            sync.RWMutex
	executions    map[IdempotencyKey]*Execution
	cleanupTicker *time.Ticker
	ttl           time.Duration
}

type Execution struct {
	ExecutionID string          `json:"executionId"`
	Result      json.RawMessage `json:"result,omitempty"`
	Failures    []error         `json:"error,omitempty"`
	State       ExecutionState  `json:"state"`
	Timestamp   time.Time       `json:"timestamp"`
	StartedAt   time.Time       `json:"startedAt"`
	LeaseExpiry time.Time       `json:"leaseExpiry"`
}

func (e *Execution) pushFailure(err error) {
	e.Failures = append(e.Failures, err)
}

func (e *Execution) GetFailure(index int) (error, bool) {
	if index >= 0 && index < len(e.Failures) {
		return e.Failures[index], true
	}
	return nil, false
}

func NewIdempotencyStore(ttl time.Duration) *IdempotencyStore {
	if ttl == 0 {
		ttl = defaultStoreTTL
	}

	store := &IdempotencyStore{
		executions:    make(map[IdempotencyKey]*Execution),
		cleanupTicker: time.NewTicker(1 * time.Hour),
		ttl:           ttl,
	}

	go store.startCleanup()
	return store
}

func (s *IdempotencyStore) InitializeExecution(key IdempotencyKey, executionID string) (*Execution, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()

	if execution, exists := s.executions[key]; exists {
		switch execution.State {
		case ExecutionCompleted, ExecutionFailed:
			return execution, false, nil

		case ExecutionPaused:
			execution.State = ExecutionInProgress
			execution.ExecutionID = executionID
			execution.StartedAt = now
			execution.LeaseExpiry = now.Add(defaultLeaseDuration)
			return execution, true, nil

		case ExecutionInProgress:
			if now.After(execution.LeaseExpiry) {
				execution.ExecutionID = executionID
				execution.StartedAt = now
				execution.LeaseExpiry = now.Add(defaultLeaseDuration)
				return execution, true, nil
			}

			// Execution still valid
			return execution, false, nil
		}
	}

	// New execution
	newExecution := &Execution{
		ExecutionID: executionID,
		State:       ExecutionInProgress,
		Timestamp:   now,
		StartedAt:   now,
		LeaseExpiry: now.Add(defaultLeaseDuration),
	}
	s.executions[key] = newExecution
	return newExecution, true, nil
}

func (s *IdempotencyStore) RenewLease(key IdempotencyKey, executionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if execution, exists := s.executions[key]; exists &&
		execution.State == ExecutionInProgress &&
		execution.ExecutionID == executionID {
		execution.LeaseExpiry = time.Now().UTC().Add(defaultLeaseDuration)
		return true
	}
	return false
}

func (s *IdempotencyStore) PauseExecution(key IdempotencyKey) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if execution, exists := s.executions[key]; exists &&
		execution.State == ExecutionInProgress {
		execution.State = ExecutionPaused
		execution.LeaseExpiry = time.Time{} // Clear lease
	}
}

func (s *IdempotencyStore) ResumeExecution(key IdempotencyKey) (*Execution, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if execution, exists := s.executions[key]; exists &&
		execution.State == ExecutionPaused {
		execution.State = ExecutionInProgress
		execution.LeaseExpiry = time.Now().UTC().Add(defaultLeaseDuration)
		return execution, true
	}
	return nil, false
}

func (s *IdempotencyStore) ResetFailedExecution(key IdempotencyKey) (*Execution, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if execution, exists := s.executions[key]; exists &&
		execution.State == ExecutionFailed {
		execution.State = ExecutionInProgress
		execution.LeaseExpiry = time.Now().UTC().Add(defaultLeaseDuration)
		return execution, true
	}
	return nil, false
}

func (s *IdempotencyStore) UpdateExecutionResult(key IdempotencyKey, result json.RawMessage, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if execution, exists := s.executions[key]; exists {
		execution.Result = result
		execution.pushFailure(err)
		execution.State = ExecutionCompleted
		if err != nil {
			execution.State = ExecutionFailed
		}
		execution.Timestamp = time.Now().UTC()
	}
}

func (s *IdempotencyStore) GetExecutionWithResult(key IdempotencyKey) (*Execution, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if result, exists := s.executions[key]; exists {
		return &Execution{
			ExecutionID: result.ExecutionID,
			Result:      result.Result,
			Failures:    result.Failures,
			State:       result.State,
			Timestamp:   result.Timestamp,
			StartedAt:   result.StartedAt,
			LeaseExpiry: result.LeaseExpiry,
		}, true
	}
	return nil, false
}

func (s *IdempotencyStore) startCleanup() {
	for range s.cleanupTicker.C {
		s.cleanup()
	}
}

func (s *IdempotencyStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	threshold := time.Now().UTC().Add(-s.ttl)
	for key, execution := range s.executions {
		if execution.Timestamp.Before(threshold) {
			delete(s.executions, key)
		}
	}
}

func (s *IdempotencyStore) ClearResult(key IdempotencyKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.executions, key)
}
