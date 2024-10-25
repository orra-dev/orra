package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

func NewLogManager(_ context.Context, retention time.Duration, controlPlane *ControlPlane) *LogManager {
	lm := &LogManager{
		logs:           make(map[string]*Log),
		orchestrations: make(map[string]*OrchestrationState),
		retention:      retention,
		cleanupTicker:  time.NewTicker(5 * time.Minute),
		controlPlane:   controlPlane,
	}

	//go lm.startCleanup(ctx)
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

func (lm *LogManager) MarkTask(orchestrationID, taskID string, s Status) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	state, ok := lm.orchestrations[orchestrationID]
	if !ok {
		return fmt.Errorf("orchestration %s has no associated state", orchestrationID)
	}
	state.TasksStatuses[taskID] = s
	state.LastUpdated = time.Now().UTC()

	return nil
}

func (lm *LogManager) MarkTaskCompleted(orchestrationID, taskID string) error {
	return lm.MarkTask(orchestrationID, taskID, Completed)
}

func (lm *LogManager) MarkOrchestration(orchestrationID string, s Status, reason json.RawMessage) (Status, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	state, ok := lm.orchestrations[orchestrationID]
	if !ok {
		return state.Status, fmt.Errorf("orchestration %s has no associated state", orchestrationID)
	}

	if reason != nil {
		state.Error = string(reason)
	}
	state.Status = s
	state.LastUpdated = time.Now().UTC()

	return state.Status, nil
}

func (lm *LogManager) MarkOrchestrationCompleted(orchestrationID string) (Status, error) {
	return lm.MarkOrchestration(orchestrationID, Completed, nil)
}

func (lm *LogManager) MarkOrchestrationFailed(orchestrationID string, reason json.RawMessage) (Status, error) {
	return lm.MarkOrchestration(orchestrationID, Failed, reason)
}

func (lm *LogManager) GetOrchestrationProjectID(orchestrationID string) string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.orchestrations[orchestrationID].ProjectID
}

func (lm *LogManager) AppendToLog(orchestrationID, entryType, id string, reason json.RawMessage, producerID string) {
	// Create a new log entry for our task's output
	newEntry := NewLogEntry(entryType, id, reason, producerID, 0)

	log, exists := lm.logs[orchestrationID]
	if !exists {
		lm.Logger.Info().
			Str("Orchestration", orchestrationID).
			Msg("Cannot append to Log, orchestration may have been already finalised.")
		return
	}

	log.Append(newEntry)
}

func (lm *LogManager) AppendFailureToLog(orchestrationID, id, producerID, reason string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	reasonData, err := json.Marshal(reason)
	if err != nil {
		return fmt.Errorf("failed to marshal reason for log entry: %w", err)
	}

	lm.AppendToLog(orchestrationID, "task_failure", id, reasonData, producerID)
	return nil
}

func (lm *LogManager) FinalizeOrchestration(orchestrationID string, status Status, reason, result json.RawMessage) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if err := lm.controlPlane.FinalizeOrchestration(orchestrationID, status, reason, []json.RawMessage{result}); err != nil {
		return err
	}

	delete(lm.logs, orchestrationID)
	delete(lm.orchestrations, orchestrationID)

	return nil
}

func (o *OrchestrationState) GetSubTasksFor(serviceID string) map[string]SubTask {
	out := map[string]SubTask{}
	for _, subTask := range o.Plan.Tasks {
		if strings.EqualFold(subTask.Service, serviceID) {
			out[subTask.ID] = SubTask{
				ID:             subTask.ID,
				Service:        subTask.Service,
				ServiceDetails: subTask.ServiceDetails,
				Input:          subTask.Input,
				Status:         subTask.Status,
				Error:          subTask.Error,
			}
		}
	}
	return out
}

func (lm *LogManager) GetActiveOrchestrationsWithTasks(projectID, serviceID string) map[string]map[string]SubTask {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	out := make(map[string]map[string]SubTask)

	lm.Logger.Debug().Interface("lm.orchestrations", lm.orchestrations).Msg("current orchestrations")

	for oID, o := range lm.orchestrations {
		lm.Logger.Debug().
			Str("oID", oID).
			Str("projectID", projectID).
			Str("o.ProjectID", o.ProjectID).
			Str("o.Status", o.Status.String()).
			Msg("current orchestrations")
		if projectID == o.ProjectID && o.Status != Completed {
			out[oID] = o.GetSubTasksFor(serviceID)
		}
	}

	return out
}

func (lm *LogManager) UpdateActiveOrchestrations(orchestrationsAndTasks map[string]map[string]SubTask, serviceID, reason string, oldStatus, newStatus Status) {
	for orchestrationID, tasks := range orchestrationsAndTasks {
		err := lm.UpdateOrchestrationStatus(orchestrationID, tasks, serviceID, reason, oldStatus, newStatus)
		if err != nil {
			lm.Logger.Error().Err(err).Fields(map[string]any{
				"orchestrationId": orchestrationID,
				"serviceId":       serviceID,
				"tasks":           tasks,
				"reason":          reason,
				"oldStatus":       oldStatus,
				"newStatus":       newStatus,
			}).Msg("Failed to notify orchestration of new status")
			continue
		}
	}
}

func (lm *LogManager) UpdateOrchestrationStatus(orchestrationID string, tasks map[string]SubTask, serviceID, reason string, oldStatus, newStatus Status) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.Logger.Debug().
		Str("expectedOldStatus", oldStatus.String()).
		Str("newIntendedStatus", newStatus.String()).
		Fields(tasks).
		Msgf("Updating orchestration status: %s", orchestrationID)

	state, exists := lm.orchestrations[orchestrationID]
	if !exists {
		return fmt.Errorf("orchestration %s not found", orchestrationID)
	}

	if state.Status != oldStatus {
		lm.Logger.Debug().
			Str("expectedOldStatus", oldStatus.String()).
			Str("actualOldStatus", state.Status.String()).
			Str("newIntendedStatus", newStatus.String()).
			Msgf("Ignoring orchestration status update for orchestration %s", orchestrationID)
		return nil
	}

	state.Status = newStatus
	var taskIDs []string
	for taskID := range tasks {
		if state.TasksStatuses[taskID] == Completed {
			continue
		}
		state.TasksStatuses[taskID] = newStatus
		taskIDs = append(taskIDs, taskID)
	}
	state.LastUpdated = time.Now().UTC()

	// Create a unique ID for the status change entry
	entryID := fmt.Sprintf("status_change_%s_%s", orchestrationID, uuid.New().String())

	var statusChange = struct {
		OrchestrationID string   `json:"orchestrationID"`
		TaskIDs         []string `json:"tasks"`
		ServiceID       string   `json:"serviceID"`
		OldStatus       Status   `json:"oldStatus"`
		NewStatus       Status   `json:"newStatus"`
		Reason          string   `json:"reason"`
	}{
		OrchestrationID: orchestrationID,
		TaskIDs:         taskIDs,
		ServiceID:       serviceID,
		OldStatus:       oldStatus,
		NewStatus:       newStatus,
		Reason:          reason,
	}

	message, err := json.Marshal(statusChange)
	if err != nil {
		return fmt.Errorf("failed to marshal status change for orchestration %s: %w", orchestrationID, err)
	}

	lm.AppendToLog(orchestrationID, "orchestration_status_change", entryID, message, "log_manager")
	return nil
}

func (lm *LogManager) GetOrchestrationStatus(orchestrationID string) (Status, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	state, exists := lm.orchestrations[orchestrationID]
	if !exists {
		return 0, fmt.Errorf("orchestration %s not found", orchestrationID)
	}

	return state.Status, nil
}

func (lm *LogManager) IsOrchestrationPaused(orchestrationID string) (bool, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	state, exists := lm.orchestrations[orchestrationID]
	if !exists {
		return false, fmt.Errorf("orchestration %s not found", orchestrationID)
	}

	return state.Status == Paused, nil
}

func (lm *LogManager) IsTaskPaused(orchestrationID, taskID string) bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	state, exists := lm.orchestrations[orchestrationID]
	if !exists {
		return false
	}

	return state.TasksStatuses[taskID] == Paused
}

func (lm *LogManager) IsTaskCompleted(orchestrationID, taskID string) (bool, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	state, exists := lm.orchestrations[orchestrationID]
	if !exists {
		return false, fmt.Errorf("orchestration %s not found", orchestrationID)
	}

	return state.TasksStatuses[taskID] == Completed, nil
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
