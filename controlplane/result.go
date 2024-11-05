package main

import (
	"context"
	"encoding/json"
	"time"
)

func NewResultAggregator(dependencies DependencyKeys, logManager *LogManager) LogWorker {
	return &ResultAggregator{
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
			var processableEntries []LogEntry

			entries := logStream.ReadFrom(r.logState.LastOffset)
			for _, entry := range entries {
				if !r.shouldProcess(entry) {
					continue
				}

				processableEntries = append(processableEntries, entry)
				select {
				case entriesChan <- entry:
					r.logState.LastOffset = entry.Offset() + 1
				case <-ctx.Done():
					return
				}
			}

			//r.LogManager.Logger.Debug().
			//	Interface("entries", processableEntries).
			//	Msgf("polling log entries for result aggregator in orchestration: %s", orchestrationID)

		case <-ctx.Done():
			return
		}
	}
}

func (r *ResultAggregator) shouldProcess(entry LogEntry) bool {
	_, isDependency := r.Dependencies[entry.ID()]
	return entry.Type() == "task_output" && isDependency
}

func (r *ResultAggregator) processEntry(entry LogEntry, orchestrationID string) error {
	if _, exists := r.logState.DependencyState[entry.ID()]; exists {
		return nil
	}

	if entry.Value() == nil {
		return nil
	}

	// Store the entry's output in our dependency state
	r.logState.DependencyState[entry.ID()] = entry.Value()

	if !containsAll(r.logState.DependencyState, r.Dependencies) {
		return nil
	}

	r.LogManager.Logger.Debug().
		Msgf("All result aggregator dependencies have been processed for orchestration: %s", orchestrationID)

	if err := r.LogManager.MarkTaskCompleted(orchestrationID, entry.ID(), time.Now().UTC()); err != nil {
		return r.LogManager.AppendFailureToLog(orchestrationID, ResultAggregatorID, ResultAggregatorID, err.Error(), 0)
	}

	completed, err := r.LogManager.MarkOrchestrationCompleted(orchestrationID)
	if err != nil {
		return r.LogManager.AppendFailureToLog(orchestrationID, ResultAggregatorID, ResultAggregatorID, err.Error(), 0)
	}
	results := r.logState.DependencyState.SortedValues()

	return r.LogManager.FinalizeOrchestration(orchestrationID, completed, nil, results[len(results)-1])
}
