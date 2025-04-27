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
	"github.com/rs/zerolog"
)

func NewLogManager(_ context.Context, storage LogStore, retention time.Duration, engine *PlanEngine) (*LogManager, error) {
	lm := &LogManager{
		logs:           make(map[string]*Log),
		orchestrations: make(map[string]*OrchestrationState),
		retention:      retention,
		cleanupTicker:  time.NewTicker(5 * time.Minute),
		planEngine:     engine,
		storage:        storage,
	}

	if err := lm.recover(); err != nil {
		lm.Logger.Error().Err(err).Msg("Failed to recover state")
		return nil, fmt.Errorf("failed to recover state: %w", err)
	}

	// TODO: enable orchestrationState cleanup
	//go lm.startCleanup(ctx)
	return lm, nil
}

func (lm *LogManager) recover() error {
	// First load all orchestration states
	states, err := lm.storage.ListOrchestrationStates()
	if err != nil {
		return fmt.Errorf("failed to list orchestration states: %w", err)
	}

	for _, state := range states {
		// Skip expired orchestrations
		if state.Status == Pending {
			continue
		}

		if OrchestrationHasExpired(state.Status, state.LastUpdated, lm.retention) {
			continue
		}

		// Load entries for this orchestration
		entries, err := lm.storage.LoadEntries(state.ID)
		if err != nil {
			return fmt.Errorf("failed to load entries for orchestration %s: %w", state.ID, err)
		}

		// Recreate log
		log := NewLog(lm.storage, lm.Logger)
		for _, entry := range entries {
			log.Append(state.ID, entry, false)
		}

		lm.logs[state.ID] = log
		lm.orchestrations[state.ID] = state
	}

	return nil
}

func OrchestrationHasExpired(status Status, lastUpdated time.Time, retention time.Duration) bool {
	return status == Completed && time.Now().UTC().Sub(lastUpdated) > retention
}

func (lm *LogManager) GetLog(orchestrationID string) *Log {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.logs[orchestrationID]
}

func (lm *LogManager) PrepLogForOrchestration(projectID string, orchestrationID string, plan *ExecutionPlan) *Log {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	log := NewLog(lm.storage, lm.Logger)

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

func (lm *LogManager) MarkTaskAborted(orchestrationID, taskID string, timestamp time.Time) error {
	return lm.MarkTask(orchestrationID, taskID, Aborted, timestamp)
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

	if lm.storage != nil {
		if err := lm.storage.StoreState(state); err != nil {
			lm.Logger.Error().
				Err(err).
				Str("orchestrationID", orchestrationID).
				Msg("Failed to persist orchestration state - initiating shutdown")

			//go lm.initiateGracefulShutdown()
			panic(fmt.Sprintf("State persistence failure: %v", err))
		}
	}

	return nil
}

func (lm *LogManager) MarkOrchestrationCompleted(orchestrationID string) Status {
	return lm.MarkOrchestration(orchestrationID, Completed, nil)
}

func (lm *LogManager) MarkOrchestrationAborted(orchestrationID string) Status {
	return lm.MarkOrchestration(orchestrationID, Aborted, nil)
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

	// Persist state change
	if lm.storage != nil {
		if err := lm.storage.StoreState(state); err != nil {
			lm.Logger.Error().
				Err(err).
				Str("orchestrationID", orchestrationID).
				Msg("Failed to persist orchestration state - initiating shutdown")

			//go lm.initiateGracefulShutdown()
			panic(fmt.Sprintf("State persistence failure: %v", err))
		}
	}

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

	log.Append(orchestrationID, newEntry, true)
}

func (lm *LogManager) AppendTaskFailureToLog(orchestrationID, id, producerID, failure string, attemptNo int) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	var f = LoggedFailure{
		Failure: failure,
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

func (lm *LogManager) FinalizeOrchestration(projectID, orchestrationID string, status Status, reason, result, abortPayload json.RawMessage, ts time.Time) error {
	if err := lm.planEngine.FinalizeOrchestration(orchestrationID, status, reason, []json.RawMessage{result}, abortPayload); err != nil {
		return fmt.Errorf("failed to finalize orchestration: %w", err)
	}

	if !shouldCompensate(status) {
		return nil
	}
	compContext := lm.prepareCompensationContext(orchestrationID, status, reason, abortPayload, ts)
	candidates, err := lm.prepareCompensationCandidates(orchestrationID, compContext)
	if err != nil {
		return err
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

	lm.triggerCompensation(projectID, orchestrationID, candidates)

	return nil
}

func shouldCompensate(status Status) bool {
	return status == Failed || status == Aborted
}

func (lm *LogManager) prepareCompensationContext(orchestrationID string, status Status, reason json.RawMessage, abortPayload json.RawMessage, ts time.Time) *CompensationContext {
	var payload json.RawMessage
	if status == Failed {
		payload = reason
	} else {
		payload = abortPayload
	}

	return &CompensationContext{
		OrchestrationID: orchestrationID,
		Reason:          status,
		Payload:         payload,
		Timestamp:       ts,
	}
}

func (lm *LogManager) prepareCompensationCandidates(orchestrationID string, compContext *CompensationContext) ([]CompensationCandidate, error) {
	log := lm.GetLog(orchestrationID)
	entries := log.ReadFrom(0)

	var candidates []CompensationCandidate
	for _, entry := range entries {
		if entry.EntryType != CompensationDataStoredLogType {
			continue
		}

		svc, err := lm.planEngine.GetServiceByID(entry.ProducerID)
		if err != nil {
			return nil, err
		}
		if !svc.Revertible {
			continue
		}

		taskID, _ := strings.CutPrefix(entry.Id, "comp_data_")
		var compensation CompensationData
		if err := json.Unmarshal(entry.GetValue(), &compensation); err != nil {
			return nil, err
		}
		compensation.Context = compContext
		candidates = append(candidates, CompensationCandidate{
			ID:           lm.planEngine.GenerateCompensationKey(),
			TaskID:       taskID,
			Service:      svc,
			Compensation: &compensation,
		})
	}
	return candidates, nil
}

func (lm *LogManager) triggerCompensation(projectID string, orchestrationID string, candidates []CompensationCandidate) {
	ctx, cancel := context.WithCancel(context.Background())
	worker := NewCompensationWorker(projectID, orchestrationID, lm, candidates, cancel)
	go worker.Start(ctx, orchestrationID)
}

func NewLog(storage LogStore, logger zerolog.Logger) *Log {
	return &Log{
		Entries:     make([]LogEntry, 0),
		seenEntries: make(map[string]bool),
		storage:     storage,
		logger:      logger,
	}
}

// Append ensures all appends are idempotent
func (l *Log) Append(storageId string, entry LogEntry, persist bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.seenEntries[entry.GetID()] {
		return
	}

	entry.Offset = l.CurrentOffset
	l.Entries = append(l.Entries, entry)
	l.CurrentOffset += 1
	l.lastAccessed = time.Now().UTC()
	l.seenEntries[entry.GetID()] = true

	if !persist {
		return
	}

	// Persist after memory operations
	if l.storage != nil {
		if err := l.storage.StoreLogEntry(storageId, entry); err != nil {
			// Log error and trigger shutdown
			l.logger.Error().
				Err(err).
				Str("entryID", entry.GetID()).
				Msg("Failed to persist log entry - initiating shutdown")

			//go l.initiateGracefulShutdown()
			panic(fmt.Sprintf("Log persistence failure: %v", err))
		}
	}
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
		EntryType:  entryType,
		Id:         id,
		Value:      append(json.RawMessage(nil), value...), // Deep copy
		Timestamp:  time.Now().UTC(),
		ProducerID: producerID,
		AttemptNum: attemptNum,
	}
}

func (e LogEntry) GetOffset() uint64         { return e.Offset }
func (e LogEntry) GetEntryType() string      { return e.EntryType }
func (e LogEntry) GetID() string             { return e.Id }
func (e LogEntry) GetValue() json.RawMessage { return append(json.RawMessage(nil), e.Value...) }
func (e LogEntry) GetTimestamp() time.Time   { return e.Timestamp }
func (e LogEntry) GetProducerID() string     { return e.ProducerID }
func (e LogEntry) GetAttemptNum() int        { return e.AttemptNum }

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
