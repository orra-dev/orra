/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type OrchestrationView struct {
	ID           string               `json:"id"`
	Action       string               `json:"action"`
	Status       Status               `json:"status"`
	Error        json.RawMessage      `json:"error,omitempty"`
	Timestamp    time.Time            `json:"timestamp"`
	Compensation *CompensationSummary `json:"compensation,omitempty"`
}

type CompensationSummary struct {
	Active    bool `json:"active"`    // Are any compensations still running?
	Total     int  `json:"total"`     // Total number of compensatable tasks
	Completed int  `json:"completed"` // Number of completed compensations
	Failed    int  `json:"failed"`    // Number of failed compensations
}

type OrchestrationListView struct {
	Pending       []OrchestrationView `json:"pending,omitempty"`
	Processing    []OrchestrationView `json:"processing,omitempty"`
	Completed     []OrchestrationView `json:"completed,omitempty"`
	Failed        []OrchestrationView `json:"failed,omitempty"`
	NotActionable []OrchestrationView `json:"notActionable,omitempty"`
}

type OrchestrationInspectResponse struct {
	ID        string                `json:"id"`
	Status    Status                `json:"status"`
	Action    string                `json:"action"`
	Timestamp time.Time             `json:"timestamp"`
	Error     json.RawMessage       `json:"error,omitempty"`
	Tasks     []TaskInspectResponse `json:"tasks,omitempty"`
	Results   []json.RawMessage     `json:"results,omitempty"`
	Duration  time.Duration         `json:"duration"` // Time since orchestration started
}

type TaskInspectResponse struct {
	ID                  string                    `json:"id"`
	ServiceID           string                    `json:"serviceId"`
	ServiceName         string                    `json:"serviceName"` // Added service name
	Status              Status                    `json:"status"`
	StatusHistory       []TaskStatusEvent         `json:"statusHistory"`
	Input               json.RawMessage           `json:"input,omitempty"`
	Output              json.RawMessage           `json:"output,omitempty"`
	Error               string                    `json:"error,omitempty"`
	Duration            time.Duration             `json:"duration"` // Time between first Processing and last status
	InterimResults      []TaskInterimResult       `json:"interimResults,omitempty"`
	Compensation        *TaskCompensationStatus   `json:"compensation,omitempty"`
	CompensationHistory []CompensationStatusEvent `json:"compensationHistory,omitempty"`
	IsRevertible        bool                      `json:"isRevertible"`
}

type TaskInterimResult struct {
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

type TaskCompensationStatus struct {
	Status      CompensationStatus `json:"status"`       // pending, processing, completed, failed
	Attempt     int                `json:"attempt"`      // Current attempt number (1-based)
	MaxAttempts int                `json:"max_attempts"` // Maximum attempts allowed
	Timestamp   time.Time          `json:"timestamp"`    // When compensation started
}

type CompensationStatusEvent struct {
	ID          string             `json:"id"`
	TaskID      string             `json:"taskId"`
	Status      CompensationStatus `json:"status"`      // "processing", "completed", "failed"
	Attempt     int                `json:"attempt"`     // Current attempt number (1-based)
	MaxAttempts int                `json:"maxAttempts"` // Maximum allowed attempts
	Timestamp   time.Time          `json:"timestamp"`
	Error       string             `json:"error,omitempty"`
}

type taskLookupMaps struct {
	serviceNames       map[string]string
	taskOutputs        map[string]json.RawMessage
	taskStatuses       map[string][]TaskStatusEvent
	taskInterimResults map[string][]TaskInterimResult
}

type task0Values map[string]interface{}

func (p *PlanEngine) GetOrchestrationList(projectID string) OrchestrationListView {
	// Get orchestrations for this project
	orchestrations := p.getProjectOrchestrations(projectID)

	// Convert to view objects and group by status
	grouped := make(map[Status][]OrchestrationView)
	for _, o := range orchestrations {
		view := OrchestrationView{
			ID:        o.ID,
			Action:    o.Action.Content,
			Status:    o.Status,
			Error:     o.Error,
			Timestamp: o.Timestamp,
		}

		if o.Status == Failed {
			if log := p.LogManager.GetLog(o.ID); log != nil {
				entries := log.ReadFrom(0)
				view.Compensation = p.processCompensationSummary(entries, o.Plan)
			}
		}

		grouped[o.Status] = append(grouped[o.Status], view)
	}

	// Sort each group by timestamp (newest first)
	for status := range grouped {
		sort.Slice(grouped[status], func(i, j int) bool {
			return grouped[status][i].Timestamp.After(grouped[status][j].Timestamp)
		})
	}

	return OrchestrationListView{
		Pending:       grouped[Pending],
		Processing:    grouped[Processing],
		Completed:     grouped[Completed],
		Failed:        grouped[Failed],
		NotActionable: grouped[NotActionable],
	}
}

func (p *PlanEngine) processCompensationSummary(entries []LogEntry, plan *ExecutionPlan) *CompensationSummary {
	if plan == nil {
		return nil
	}

	// Count revertible services in the plan
	var revertibleTasks int
	for _, task := range plan.Tasks {
		if task.ID == TaskZero {
			continue
		}

		service, err := p.GetServiceByID(task.Service)
		if err != nil {
			continue
		}

		if service.Revertible {
			revertibleTasks++
		}
	}

	if revertibleTasks == 0 {
		return nil
	}

	summary := &CompensationSummary{
		Total: revertibleTasks,
	}

	// Track the latest state for each task
	taskStates := make(map[string]string)

	for _, entry := range entries {
		switch entry.GetEntryType() {
		case CompensationAttemptedLogType:
			summary.Active = true
			taskStates[entry.GetProducerID()] = "processing"

		case CompensationCompleteLogType, CompensationPartialLogType:
			taskStates[entry.GetProducerID()] = "completed"
			summary.Completed++

		case CompensationFailureLogType, CompensationExpiredLogType:
			taskStates[entry.GetProducerID()] = "failed"
			summary.Failed++
		}
	}

	// Check if any compensations are still active
	summary.Active = false
	for _, state := range taskStates {
		if state == "processing" {
			summary.Active = true
			break
		}
	}

	p.Logger.
		Trace().
		Interface("taskStates", taskStates).
		Msg("processCompensationSummary")

	return summary
}

func (p *PlanEngine) getProjectOrchestrations(projectID string) []*Orchestration {
	p.orchestrationStoreMu.RLock()
	defer p.orchestrationStoreMu.RUnlock()

	var result []*Orchestration
	for _, o := range p.orchestrationStore {
		if o.ProjectID == projectID {
			result = append(result, o)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	p.Logger.Trace().Interface("orchestrations", result).Msg("orchestrations for list view")
	return result
}

func (p *PlanEngine) InspectOrchestration(orchestrationID string) (*OrchestrationInspectResponse, error) {
	// Get orchestration with appropriate locking
	orchestration, err := p.getOrchestration(orchestrationID)
	if err != nil {
		return nil, err
	}

	if orchestration.FailedBeforeDecomposition() {
		return &OrchestrationInspectResponse{
			ID:        orchestration.ID,
			Status:    Failed,
			Action:    orchestration.Action.Content,
			Timestamp: orchestration.Timestamp,
			Error:     orchestration.Error,
			Duration:  time.Since(orchestration.Timestamp),
		}, nil
	}

	if orchestration.Status == NotActionable {
		return &OrchestrationInspectResponse{
			ID:        orchestration.ID,
			Status:    NotActionable,
			Action:    orchestration.Action.Content,
			Timestamp: orchestration.Timestamp,
			Error:     orchestration.Error,
			Duration:  time.Since(orchestration.Timestamp),
		}, nil
	}

	if orchestration.Status == Cancelled {
		return &OrchestrationInspectResponse{
			ID:        orchestration.ID,
			Status:    Cancelled,
			Action:    orchestration.Action.Content,
			Timestamp: orchestration.Timestamp,
			Error:     orchestration.Error,
			Duration:  time.Since(orchestration.Timestamp),
		}, nil
	}

	// Build lookup maps for constructing the response
	lookupMaps, err := p.buildLookupMaps(orchestrationID, orchestration)
	if err != nil {
		return nil, err
	}

	// Build task responses
	tasks, err := p.buildTaskResponses(orchestration, lookupMaps)
	if err != nil {
		return nil, err
	}

	p.Logger.Trace().
		Str("OrchestrationID", orchestrationID).
		Interface("Tasks", tasks).
		Msg("task responses")

	// Construct final response
	return &OrchestrationInspectResponse{
		ID:        orchestration.ID,
		Status:    orchestration.Status,
		Action:    orchestration.Action.Content,
		Timestamp: orchestration.Timestamp,
		Error:     orchestration.Error,
		Tasks:     tasks,
		Duration:  time.Since(orchestration.Timestamp),
		Results:   orchestration.Results,
	}, nil
}

func (p *PlanEngine) getOrchestration(orchestrationID string) (*Orchestration, error) {
	p.orchestrationStoreMu.RLock()
	defer p.orchestrationStoreMu.RUnlock()

	orchestration, exists := p.orchestrationStore[orchestrationID]
	if !exists {
		return nil, fmt.Errorf("orchestration %s not found", orchestrationID)
	}
	return orchestration, nil
}

func (p *PlanEngine) buildLookupMaps(orchestrationID string, orchestration *Orchestration) (*taskLookupMaps, error) {
	// Get service names
	serviceNames, err := p.getServiceNames(orchestration)
	if err != nil {
		return nil, fmt.Errorf("error getting service names: %w", err)
	}

	// Get log entries
	log := p.LogManager.GetLog(orchestrationID)
	if log == nil {
		return nil, fmt.Errorf("log not found for orchestration %s", orchestrationID)
	}

	// Process log entries to build output and status maps
	taskOutputs, taskStatuses, taskInterimResults := p.processLogEntries(log)

	return &taskLookupMaps{
		serviceNames:       serviceNames,
		taskOutputs:        taskOutputs,
		taskStatuses:       taskStatuses,
		taskInterimResults: taskInterimResults,
	}, nil
}

func (p *PlanEngine) getServiceNames(orchestration *Orchestration) (map[string]string, error) {
	serviceNames := make(map[string]string)
	for _, task := range orchestration.Plan.Tasks {
		if task.ID == "task0" {
			continue
		}
		if svc, err := p.GetServiceByID(task.Service); err == nil {
			serviceNames[task.Service] = svc.Name
		}
	}
	return serviceNames, nil
}

func (p *PlanEngine) processLogEntries(log *Log) (map[string]json.RawMessage, map[string][]TaskStatusEvent, map[string][]TaskInterimResult) {
	taskOutputs := make(map[string]json.RawMessage)
	taskStatuses := make(map[string][]TaskStatusEvent)
	interimResults := make(map[string][]TaskInterimResult)

	entries := log.ReadFrom(0)
	p.Logger.Trace().Interface("Log Entries", entries).Msg("processing log entries for orchestration inspection")
	for _, entry := range entries {
		switch entry.GetEntryType() {
		case "task_output":
			taskOutputs[entry.GetID()] = entry.GetValue()
		case "task_status":
			processStatusEntry(entry, taskStatuses, p.Logger)
		case "task_interim_output":
			processTaskInterimResultEntry(entry, interimResults)
		}
	}

	return taskOutputs, taskStatuses, interimResults
}

func processTaskInterimResultEntry(entry LogEntry, interimOutput map[string][]TaskInterimResult) {
	parts := strings.Split(entry.GetID(), "_")
	if len(parts) != 3 {
		return
	}

	taskID := parts[1]
	result := TaskInterimResult{
		ID:        entry.GetID(),
		Timestamp: entry.GetTimestamp(),
		Data:      entry.GetValue(),
	}
	interimOutput[taskID] = append(interimOutput[taskID], result)

	for taskID, results := range interimOutput {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Timestamp.Before(results[j].Timestamp)
		})
		interimOutput[taskID] = results
	}
}

func processStatusEntry(entry LogEntry, taskStatuses map[string][]TaskStatusEvent, logger zerolog.Logger) {
	var statusEvent TaskStatusEvent

	logger.Trace().RawJSON("Status event value", entry.GetValue()).Msg("")
	if err := json.Unmarshal(entry.GetValue(), &statusEvent); err != nil {
		logger.Trace().Err(err).Msg("failed to unmarshal status event value")
		return
	}

	taskID := statusEvent.TaskID
	events := taskStatuses[taskID]

	insertIdx := findStatusInsertionPoint(events, statusEvent.Timestamp)
	logger.Trace().Int("insertIdx", insertIdx).Msg("Status insertion index based on timestamp")

	skipStatus := shouldSkipDuplicateStatus(events, insertIdx, statusEvent)
	logger.Trace().
		Interface("events", events).
		Interface("statusEvent", statusEvent).
		Msgf("shouldSkipDuplicateStatus: [%t]", skipStatus)
	if skipStatus {
		return
	}

	taskStatuses[taskID] = insertStatusEvent(events, statusEvent, insertIdx)
}

func findStatusInsertionPoint(events []TaskStatusEvent, timestamp time.Time) int {
	return sort.Search(len(events), func(i int) bool {
		return events[i].Timestamp.After(timestamp)
	})
}

func shouldSkipDuplicateStatus(events []TaskStatusEvent, insertIdx int, newEvent TaskStatusEvent) bool {
	return insertIdx > 0 &&
		events[insertIdx-1].Status == newEvent.Status &&
		events[insertIdx-1].Timestamp.Equal(newEvent.Timestamp)
}

func insertStatusEvent(events []TaskStatusEvent, event TaskStatusEvent, idx int) []TaskStatusEvent {
	if idx == len(events) {
		return append(events, event)
	}
	return append(events[:idx], append(
		[]TaskStatusEvent{event},
		events[idx:]...,
	)...)
}

func (p *PlanEngine) buildTaskResponses(orchestration *Orchestration, lookupMaps *taskLookupMaps) ([]TaskInspectResponse, error) {
	var tasks []TaskInspectResponse

	for _, task := range orchestration.Plan.Tasks {
		if task.ID == "task0" {
			continue
		}

		taskResp, err := p.buildSingleTaskResponse(orchestration, task, lookupMaps)
		if err != nil {
			return nil, fmt.Errorf("error building response for task %s: %w", task.ID, err)
		}

		tasks = append(tasks, taskResp)
	}

	return tasks, nil
}

func (p *PlanEngine) buildSingleTaskResponse(
	orchestration *Orchestration,
	task *SubTask,
	lookupMaps *taskLookupMaps,
) (TaskInspectResponse, error) {
	history := lookupMaps.taskStatuses[task.ID]

	// Get the final status from the history instead of the task status
	var finalStatus Status
	if len(history) > 0 {
		finalStatus = history[len(history)-1].Status
	} else {
		// Fallback to orchestration state tracking if no history
		finalStatus = p.LogManager.orchestrations[orchestration.ID].TasksStatuses[task.ID]
	}

	taskResp := TaskInspectResponse{
		ID:            task.ID,
		ServiceID:     task.Service,
		ServiceName:   lookupMaps.serviceNames[task.Service],
		Status:        finalStatus, // Use the determined final status
		StatusHistory: history,
	}

	// Set duration if we have history
	if len(history) > 0 {
		taskResp.Duration = history[len(history)-1].Timestamp.Sub(history[0].Timestamp)
	}

	// Extract task0 values for reference resolution
	task0Vals, err := extractTask0Values(orchestration.TaskZero)
	if err != nil {
		return TaskInspectResponse{}, fmt.Errorf("failed to extract task0 values: %w", err)
	}

	// Create a copy of the input map for resolution
	inputCopy := make(map[string]interface{})
	for k, v := range task.Input {
		inputCopy[k] = v
	}

	// Resolve task0 references in the copy
	if err := resolveTask0RefsInMap(inputCopy, task0Vals); err != nil {
		return TaskInspectResponse{}, fmt.Errorf("failed to resolve task0 references: %w", err)
	}

	// Marshal resolved input
	resolvedInput, err := json.Marshal(inputCopy)
	if err != nil {
		return TaskInspectResponse{}, fmt.Errorf("error marshaling resolved input: %w", err)
	}
	taskResp.Input = resolvedInput

	// Add output if available
	if output, ok := lookupMaps.taskOutputs[task.ID]; ok {
		taskResp.Output = output
	}

	// Set error if present in last status
	if len(history) > 0 && history[len(history)-1].Error != "" {
		taskResp.Error = history[len(history)-1].Error
	}

	if interimResults, ok := lookupMaps.taskInterimResults[task.ID]; ok {
		taskResp.InterimResults = interimResults
	}

	service, err := p.GetServiceByID(task.Service)
	if err != nil {
		return TaskInspectResponse{}, fmt.Errorf("error getting service: %w", err)
	}

	taskResp.IsRevertible = service.Revertible
	log := p.LogManager.GetLog(orchestration.ID)
	if !service.Revertible {
		return taskResp, nil
	}

	taskResp.CompensationHistory = p.processCompensationHistory(
		log.ReadFrom(0),
		task.ID,
	)

	if len(taskResp.CompensationHistory) == 0 {
		return taskResp, nil
	}

	finalCompensation := taskResp.CompensationHistory[len(taskResp.CompensationHistory)-1]
	taskResp.Compensation = &TaskCompensationStatus{
		Status:      finalCompensation.Status,
		Attempt:     finalCompensation.Attempt,
		MaxAttempts: finalCompensation.MaxAttempts,
		Timestamp:   finalCompensation.Timestamp,
	}

	return taskResp, nil
}

func (p *PlanEngine) processCompensationHistory(entries []LogEntry, taskID string) []CompensationStatusEvent {
	var history []CompensationStatusEvent

	for _, entry := range entries {
		// Only process entries for this task
		if !strings.Contains(entry.GetID(), strings.ToLower(taskID)) {
			continue
		}

		var event CompensationStatusEvent
		event.TaskID = taskID
		event.Timestamp = entry.GetTimestamp()
		event.MaxAttempts = MaxCompensationAttempts

		switch entry.GetEntryType() {
		case CompensationAttemptedLogType:
			event.Status = CompensationProcessing
			event.Attempt = entry.GetAttemptNum()
			event.ID = fmt.Sprintf("comp_attempt_%s_%d", taskID, event.Attempt)
			history = append(history, event)

		case CompensationCompleteLogType:
			event.Status = CompensationCompleted
			event.Attempt = entry.GetAttemptNum()
			event.ID = fmt.Sprintf("comp_completed_%s", taskID)
			history = append(history, event)

		case CompensationPartialLogType:
			event.Status = CompensationPartial
			event.Attempt = entry.GetAttemptNum()
			event.ID = fmt.Sprintf("comp_completed_partially_%s", taskID)
			history = append(history, event)

		case CompensationExpiredLogType:
			event.Status = CompensationExpired
			event.Attempt = entry.GetAttemptNum()
			event.ID = fmt.Sprintf("comp_expired_%s", taskID)
			history = append(history, event)

		case CompensationFailureLogType:
			event.Status = CompensationFailed
			event.Attempt = entry.GetAttemptNum()
			event.ID = fmt.Sprintf("comp_fail_%s_%d", taskID, event.Attempt)

			// Extract error message if present
			var failure CompensationResult
			if err := json.Unmarshal(entry.GetValue(), &failure); err == nil {
				event.Error = failure.Error
			}

			history = append(history, event)
		}
	}

	// Sort history by timestamp
	sort.Slice(history, func(i, j int) bool {
		return history[i].Timestamp.Before(history[j].Timestamp)
	})

	p.Logger.Debug().
		Interface("history", history).
		Str("taskID", taskID).
		Msg("process compensation history")

	return history
}

func extractTask0Values(taskZeroJSON json.RawMessage) (task0Values, error) {
	var values task0Values
	if err := json.Unmarshal(taskZeroJSON, &values); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task0 values: %w", err)
	}
	return values, nil
}

func resolveTask0Ref(ref string, task0Vals task0Values) (interface{}, error) {
	// Extract field name from reference (e.g., "$task0.message" -> "message")
	matches := DependencyPattern.FindStringSubmatch(ref)
	if len(matches) != 2 || matches[1] != TaskZero {
		return ref, nil // Not a task0 reference
	}

	field := strings.TrimPrefix(ref, "$task0.")
	value, ok := task0Vals[field]
	if !ok {
		return nil, fmt.Errorf("task0 field not found: %s", field)
	}
	return value, nil
}

func resolveTask0RefsInMap(input map[string]interface{}, task0Vals task0Values) error {
	for key, value := range input {
		switch v := value.(type) {
		case string:
			if strings.HasPrefix(v, "$task0.") {
				resolved, err := resolveTask0Ref(v, task0Vals)
				if err != nil {
					return err
				}
				input[key] = resolved
			}
		case map[string]interface{}:
			if err := resolveTask0RefsInMap(v, task0Vals); err != nil {
				return err
			}
		case []interface{}:
			for i, item := range v {
				if strItem, ok := item.(string); ok && strings.HasPrefix(strItem, "$task0.") {
					resolved, err := resolveTask0Ref(strItem, task0Vals)
					if err != nil {
						return err
					}
					v[i] = resolved
				}
			}
		}
	}
	return nil
}
