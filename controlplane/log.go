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
	"sort"
	"strings"
	"time"

	short "github.com/lithammer/shortuuid/v4"
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

func (lm *LogManager) AppendTaskFailureToLog(orchestrationID, id, producerID, failure string, attemptNo int, skipWebhook bool) error {
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

	failureID := fmt.Sprintf("task_fail_%s_%s", strings.ToLower(id), short.New())
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

	eventID := fmt.Sprintf("evt_%s_%s", strings.ToLower(taskID), short.New())
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
func (lm *LogManager) AppendCompensationDataStored(orchestrationID string, taskID string, serviceID string, data *CompensationData) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	value, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal compensation data: %w", err)
	}

	lm.AppendToLog(
		orchestrationID,
		CompensationDataStoredLogType,
		fmt.Sprintf("comp_data_%s", strings.ToLower(taskID)),
		value,
		serviceID,
		0,
	)
	return nil
}

// AppendCompensationAttempted creates a log entry for a compensation attempt
func (lm *LogManager) AppendCompensationAttempted(
	orchestrationID string,
	taskID string,
	uuid string,
	attemptNo int,
) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	attemptId := fmt.Sprintf("comp_attempt_%s_%s", strings.ToLower(taskID), uuid)
	compensationAttempt := struct {
		ID        string    `json:"id"`
		TaskID    string    `json:"taskId"`
		Timestamp time.Time `json:"timestamp"`
	}{
		ID:        attemptId,
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
	}

	attemptData, err := json.Marshal(compensationAttempt)
	if err != nil {
		return fmt.Errorf("failed to marshal compensation attempt metadata: %w", err)
	}

	lm.Logger.Debug().
		Str("AttemptId", attemptId).
		Int("AttemptNo", attemptNo).
		Str("OrchestrationID", orchestrationID).
		Msgf("Appending compensation attempt for task: [%s]", taskID)

	lm.AppendToLog(
		orchestrationID,
		CompensationAttemptedLogType,
		attemptId,
		attemptData,
		taskID,
		attemptNo,
	)
	return nil
}

// AppendCompensationComplete creates a log entry for a completed compensation
func (lm *LogManager) AppendCompensationComplete(
	orchestrationID string,
	taskID string,
	logType string,
	result *CompensationResult,
	attemptNo int,
) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	compensationCompleted := struct {
		*CompensationResult
		Timestamp time.Time `json:"timestamp"`
	}{
		CompensationResult: result,
		Timestamp:          time.Now().UTC(),
	}

	compCompleted, err := json.Marshal(compensationCompleted)
	if err != nil {
		return fmt.Errorf("failed to marshal compensation result: %w", err)
	}

	lm.AppendToLog(
		orchestrationID,
		logType,
		fmt.Sprintf("comp_complete_%s", strings.ToLower(taskID)),
		compCompleted,
		taskID,
		attemptNo,
	)
	return nil
}

func (lm *LogManager) AppendCompensationFailure(
	orchestrationID,
	taskID string,
	logType string,
	failure CompensationResult,
	attemptNo int,
) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	value, err := json.Marshal(failure)
	if err != nil {
		return fmt.Errorf("failed to marshal compensation failure for log entry: %w", err)
	}

	failureID := fmt.Sprintf("comp_fail_%s", strings.ToLower(taskID))
	lm.AppendToLog(
		orchestrationID,
		logType,
		failureID,
		value,
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
	if err := lm.controlPlane.FinalizeOrchestration(orchestrationID, status, reason, []json.RawMessage{result}, skipWebhook); err != nil {
		return fmt.Errorf("failed to finalize orchestration: %w", err)
	}

	if status != Failed {
		return nil
	}

	log := lm.GetLog(orchestrationID)
	entries := log.ReadFrom(0)

	var candidates []CompensationCandidate
	for _, entry := range entries {
		if entry.entryType != CompensationDataStoredLogType {
			continue
		}

		svc, err := lm.controlPlane.GetServiceByID(entry.producerID)
		if err != nil {
			return err
		}
		if !svc.Revertible {
			continue
		}

		taskID, _ := strings.CutPrefix(entry.id, "comp_data_")
		var compensation CompensationData
		if err := json.Unmarshal(entry.Value(), &compensation); err != nil {
			return err
		}
		candidates = append(candidates, CompensationCandidate{
			TaskID:       taskID,
			Service:      svc,
			Compensation: &compensation,
		})
	}

	if len(candidates) == 0 {
		lm.Logger.Info().
			Str("OrchestrationID", orchestrationID).
			Msg("Orchestration has no completed tasks to compensate")
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].TaskID > candidates[j].TaskID
	})

	lm.Logger.Trace().
		Interface("CompensationCandidates", candidates).
		Str("OrchestrationID", orchestrationID).
		Msg("Preparing sorted compensation candidates")

	lm.triggerCompensation(orchestrationID, candidates)

	return nil
}

func (lm *LogManager) triggerCompensation(orchestrationID string, candidates []CompensationCandidate) {
	ctx, cancel := context.WithCancel(context.Background())
	worker := NewCompensationWorker(orchestrationID, lm, candidates, cancel)
	go worker.Start(ctx, orchestrationID)
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
