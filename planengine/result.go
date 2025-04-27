/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"encoding/json"
	"time"
)

func NewResultAggregator(projectID string, dependencies DependencyKeySet, logManager *LogManager) LogWorker {
	return &ResultAggregator{
		ProjectID:    projectID,
		Dependencies: dependencies,
		LogManager:   logManager,
		logState: &LogState{
			LastOffset:      0,
			Processed:       make(map[string]bool),
			DependencyState: make(map[string]json.RawMessage),
		},
	}
}

func (r *ResultAggregator) Start(ctx context.Context, orchestrationID string) {
	logStream := r.LogManager.GetLog(orchestrationID)
	if logStream == nil {
		r.LogManager.Logger.Error().Str("orchestrationID", orchestrationID).Msg("Log stream not found for orchestration")
		return
	}

	// Channel to receive new log entries
	entriesChan := make(chan LogEntry, 100)

	// Start a goroutine for continuous polling
	go r.PollLog(ctx, orchestrationID, logStream, entriesChan)

	// Process entries as they come in
	for {
		select {
		case entry := <-entriesChan:
			if err := r.processEntry(entry, orchestrationID); err != nil {
				r.LogManager.Logger.
					Error().
					Err(err).
					Str("orchestrationID", orchestrationID).
					Interface("entry", entry).
					Msg("Result aggregator failed to process entry for orchestration")
				return
			}
		case <-ctx.Done():
			r.LogManager.Logger.Info().Msgf("Result aggregator in orchestration %s is stopping", orchestrationID)
			return
		}
	}
}

func (r *ResultAggregator) PollLog(ctx context.Context, _ string, logStream *Log, entriesChan chan<- LogEntry) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			entries := logStream.ReadFrom(r.logState.LastOffset)
			for _, entry := range entries {
				if !r.shouldProcess(entry) {
					continue
				}

				select {
				case entriesChan <- entry:
					r.logState.LastOffset = entry.GetOffset() + 1
				case <-ctx.Done():
					return
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

func (r *ResultAggregator) shouldProcess(entry LogEntry) bool {
	_, isDependency := r.Dependencies[entry.GetID()]
	return entry.GetEntryType() == "task_output" && isDependency
}

func (r *ResultAggregator) processEntry(entry LogEntry, orchestrationID string) error {
	if _, exists := r.logState.DependencyState[entry.GetID()]; exists {
		return nil
	}

	if entry.GetValue() == nil {
		return nil
	}

	// Store the entry's output in our dependency state
	r.logState.DependencyState[entry.GetID()] = entry.GetValue()

	if !resultDependenciesMet(r.logState.DependencyState, r.Dependencies) {
		return nil
	}

	r.LogManager.Logger.Debug().
		Msgf("All result aggregator dependencies have been processed for orchestration: %s", orchestrationID)

	if err := r.LogManager.MarkTaskCompleted(orchestrationID, entry.GetID(), time.Now().UTC()); err != nil {
		return r.LogManager.AppendTaskFailureToLog(orchestrationID, ResultAggregatorID, ResultAggregatorID, err.Error(), 0)
	}

	completed := r.LogManager.MarkOrchestrationCompleted(orchestrationID)
	results := r.logState.DependencyState.SortedValues()

	if err := r.LogManager.FinalizeOrchestration(r.ProjectID, orchestrationID, completed, nil, results[len(results)-1], nil, entry.GetTimestamp()); err != nil {
		return r.LogManager.AppendTaskFailureToLog(orchestrationID, ResultAggregatorID, ResultAggregatorID, err.Error(), 0)
	}

	return nil
}

func resultDependenciesMet(s map[string]json.RawMessage, e DependencyKeySet) bool {
	for srcId := range e {
		if _, hasOutput := s[srcId]; !hasOutput {
			return false
		}
	}
	return true
}
