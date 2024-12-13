/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lithammer/shortuuid/v4"
)

func NewLogManager(ctx context.Context, retention time.Duration, controlPlane *ControlPlane) *LogManager {
	lm := &LogManager{
		logs:           make(map[string]*Log),
		orchestrations: make(map[string]*OrchestrationState),
		retention:      retention,
		cleanupTicker:  time.NewTicker(5 * time.Minute),
		controlPlane:   controlPlane,
	}

	go lm.startCleanup(ctx)
	return lm
}

func (lm *LogManager) startCleanup(ctx context.Context) {
	for {
		select {
		case <-lm.cleanupTicker.C:
			lm.cleanupStaleOrchestrations()
		case <-ctx.Done():
			return
		}
	}
}

func (lm *LogManager) cleanupStaleOrchestrations() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	now := time.Now().UTC()

	for id, orchestrationState := range lm.orchestrations {
		if orchestrationState.Status == Completed &&
			now.Sub(orchestrationState.LastUpdated) > lm.retention {
			delete(lm.orchestrations, id)
			delete(lm.logs, id)
		}
	}
}

func (lm *LogManager) GetLog(orchestrationID string) *Log {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.logs[orchestrationID]
}

func (lm *LogManager) PrepLogForOrchestration(projectID string, orchestrationID string, plan *ServiceCallingPlan) *Log {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	log := NewLog()

	state := &OrchestrationState{
		ID:            orchestrationID,
		ProjectID:     projectID,
		Plan:          plan,
		TasksStatuses: make(map[string]Status),
		Status:        Processing,
		CreatedAt:     time.Now().UTC(),
	}

	lm.logs[orchestrationID] = log
	lm.orchestrations[orchestrationID] = state

	lm.Logger.Debug().Msgf("Created Log for orchestration: %s", orchestrationID)

	return log
}

func (lm *LogManager) MarkTaskCompleted(orchestrationID, taskID string, timestamp time.Time) error {
	return lm.MarkTask(orchestrationID, taskID, Completed, timestamp)
}

func (lm *LogManager) MarkTask(orchestrationID, taskID string, s Status, timestamp time.Time) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	state, ok := lm.orchestrations[orchestrationID]
	if !ok {
		return fmt.Errorf("orchestration %s has no associated state", orchestrationID)
	}

	state.TasksStatuses[taskID] = s
	state.LastUpdated = timestamp
	return nil
}

func (lm *LogManager) MarkOrchestrationCompleted(orchestrationID string) Status {
	return lm.MarkOrchestration(orchestrationID, Completed, nil)
}

func (lm *LogManager) MarkOrchestrationFailed(orchestrationID string, reason string) Status {
	return lm.MarkOrchestration(orchestrationID, Failed, []byte(reason))
}

func (lm *LogManager) MarkOrchestration(orchestrationID string, s Status, reason json.RawMessage) Status {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	state, ok := lm.orchestrations[orchestrationID]
	if !ok {
		lm.Logger.Debug().Str("OrchestrationID", orchestrationID).Msg("orchestration has no associated state in log")
		return state.Status
	}

	if reason != nil {
		state.Error = string(reason)
	}
	state.Status = s
	state.LastUpdated = time.Now().UTC()

	return state.Status
}

func (lm *LogManager) AppendToLog(orchestrationID, entryType, id string, value json.RawMessage, producerID string, attemptNo int) {
	// Create a new log entry for our task's output
	newEntry := NewLogEntry(entryType, id, value, producerID, attemptNo)

	log, exists := lm.logs[orchestrationID]
	if !exists {
		lm.Logger.Info().
			Str("Orchestration", orchestrationID).
			Msg("Cannot append to Log, orchestration may have been already finalised.")
		return
	}

	log.Append(newEntry)
}

func (lm *LogManager) AppendFailureToLog(orchestrationID, id, producerID, failure string, attemptNo int, skipWebhook bool) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	var f = LoggedFailure{
		Failure:     failure,
		SkipWebhook: skipWebhook,
	}

	value, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("failed to marshal failure for log entry: %w", err)
	}

	failureID := fmt.Sprintf("fail_%s_%s", strings.ToLower(id), shortuuid.New())
	lm.AppendToLog(orchestrationID, "task_failure", failureID, value, producerID, attemptNo)
	return nil
}

func (lm *LogManager) AppendTaskStatusEvent(
	orchestrationID,
	taskID,
	serviceID string,
	status Status,
	err error,
	timestamp time.Time,
	attemptNo int,
) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	eventID := fmt.Sprintf("evt_%s_%s", strings.ToLower(taskID), shortuuid.New())
	event := TaskStatusEvent{
		ID:              eventID,
		OrchestrationID: orchestrationID,
		TaskID:          taskID,
		Status:          status,
		Timestamp:       timestamp,
		ServiceID:       serviceID,
	}

	if err != nil {
		event.Error = err.Error()
	}

	// Create a new log entry
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry task status event: %w", err)
	}

	// Append to log with new task_status type
	lm.AppendToLog(orchestrationID, "task_status", eventID, eventData, taskID, attemptNo)

	return nil
}

// AppendCompensationDataStored creates a log entry for stored compensation data
func (lm *LogManager) AppendCompensationDataStored(
	orchestrationID string,
	taskID string,
	data *CompensationData,
) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal compensation data: %w", err)
	}

	lm.AppendToLog(
		orchestrationID,
		CompensationDataStored,
		fmt.Sprintf("comp_%s_%s", taskID, shortuuid.New()),
		dataBytes,
		taskID,
		0,
	)
	return nil
}

// AppendCompensationAttempted creates a log entry for a compensation attempt
func (lm *LogManager) AppendCompensationAttempted(
	orchestrationID string,
	taskID string,
	attemptNo int,
) error {
	meta := struct {
		TaskID    string    `json:"taskId"`
		Timestamp time.Time `json:"timestamp"`
	}{
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
	}

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal compensation attempt metadata: %w", err)
	}

	lm.AppendToLog(
		orchestrationID,
		CompensationAttempted,
		fmt.Sprintf("comp_attempt_%s_%s", taskID, shortuuid.New()),
		metaBytes,
		taskID,
		attemptNo,
	)
	return nil
}

// AppendCompensationComplete creates a log entry for a completed compensation
func (lm *LogManager) AppendCompensationComplete(
	orchestrationID string,
	taskID string,
	result *CompensationResult,
	attemptNo int,
) error {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal compensation result: %w", err)
	}

	lm.AppendToLog(
		orchestrationID,
		CompensationComplete,
		fmt.Sprintf("comp_complete_%s_%s", taskID, shortuuid.New()),
		resultBytes,
		taskID,
		attemptNo,
	)
	return nil
}

func (lm *LogManager) FinalizeOrchestration(
	orchestrationID string,
	status Status,
	reason, result json.RawMessage,
	skipWebhook bool,
) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	return lm.controlPlane.FinalizeOrchestration(orchestrationID, status, reason, []json.RawMessage{result}, skipWebhook)
}

func NewLog() *Log {
	return &Log{
		Entries:     make([]LogEntry, 0),
		seenEntries: make(map[string]bool),
	}
}

// Append ensures all appends are idempotent
func (l *Log) Append(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.seenEntries[entry.ID()] {
		return
	}

	entry.offset = l.CurrentOffset
	l.Entries = append(l.Entries, entry)
	l.CurrentOffset += 1
	l.lastAccessed = time.Now().UTC()
	l.seenEntries[entry.ID()] = true

	return
}

func (l *Log) ReadFrom(offset uint64) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if offset >= l.CurrentOffset {
		return nil
	}

	return append([]LogEntry(nil), l.Entries[offset:]...)
}

func NewLogEntry(entryType, id string, value json.RawMessage, producerID string, attemptNum int) LogEntry {
	return LogEntry{
		entryType:  entryType,
		id:         id,
		value:      append(json.RawMessage(nil), value...), // Deep copy
		timestamp:  time.Now().UTC(),
		producerID: producerID,
		attemptNum: attemptNum,
	}
}

func (e LogEntry) Offset() uint64         { return e.offset }
func (e LogEntry) Type() string           { return e.entryType }
func (e LogEntry) ID() string             { return e.id }
func (e LogEntry) Value() json.RawMessage { return append(json.RawMessage(nil), e.value...) }
func (e LogEntry) Timestamp() time.Time   { return e.timestamp }
func (e LogEntry) ProducerID() string     { return e.producerID }
func (e LogEntry) AttemptNum() int        { return e.attemptNum }

func (e LogEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Offset     uint64          `json:"offset"`
		Type       string          `json:"type"`
		ID         string          `json:"id"`
		Value      json.RawMessage `json:"value"`
		Timestamp  time.Time       `json:"timestamp"`
		ProducerID string          `json:"producerId"`
		AttemptNum int             `json:"attemptNum"`
	}{
		Offset:     e.offset,
		Type:       e.entryType,
		ID:         e.id,
		Value:      e.value,
		Timestamp:  e.timestamp,
		ProducerID: e.producerID,
		AttemptNum: e.attemptNum,
	})
}

func (d DependencyState) SortedValues() []json.RawMessage {
	var out []json.RawMessage
	var keys []string

	for key := range d {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		out = append(out, d[key])
	}
	return out
}
