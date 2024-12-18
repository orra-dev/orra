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
	"strings"
	"time"
)

func NewFailureTracker(logManager *LogManager) LogWorker {
	return &FailureTracker{
		LogManager: logManager,
		logState: &LogState{
			LastOffset: 0,
			Processed:  make(map[string]bool),
		},
	}
}

func (f *FailureTracker) Start(ctx context.Context, orchestrationID string) {
	logStream := f.LogManager.GetLog(orchestrationID)
	if logStream == nil {
		f.LogManager.Logger.Error().Str("orchestrationID", orchestrationID).Msg("Log stream not found for orchestration")
		return
	}

	// Channel to receive new log entries
	entriesChan := make(chan LogEntry, 100)

	// Start a goroutine for continuous polling
	go f.PollLog(ctx, orchestrationID, logStream, entriesChan)

	// Process entries as they come in
	for {
		select {
		case entry := <-entriesChan:
			if err := f.processEntry(entry, orchestrationID); err != nil {
				f.LogManager.Logger.
					Error().
					Err(err).
					Str("orchestrationID", orchestrationID).
					Interface("entry", entry).
					Msg("Failure tracker failed to process entry for orchestration.")
				return
			}
		case <-ctx.Done():
			f.LogManager.Logger.Info().Msgf("Failure tracker in orchestration %s is stopping", orchestrationID)
			return
		}
	}
}

func (f *FailureTracker) PollLog(ctx context.Context, _ string, logStream *Log, entriesChan chan<- LogEntry) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var processableEntries []LogEntry

			entries := logStream.ReadFrom(f.logState.LastOffset)
			for _, entry := range entries {
				if !f.shouldProcess(entry) {
					continue
				}

				processableEntries = append(processableEntries, entry)
				select {
				case entriesChan <- entry:
					f.logState.LastOffset = entry.Offset() + 1
				case <-ctx.Done():
					return
				}
			}

			//f.LogManager.Logger.Debug().
			//	Interface("entries", processableEntries).
			//	Msgf("polling entries for failure tracker in orchestration: %s", orchestrationID)
		case <-ctx.Done():
			return
		}
	}
}

func (f *FailureTracker) shouldProcess(entry LogEntry) bool {
	return entry.Type() == "task_failure"
}

func (f *FailureTracker) processEntry(entry LogEntry, orchestrationID string) error {
	// Mark this entry as processed
	f.logState.Processed[entry.ID()] = true

	var failure LoggedFailure
	if err := json.Unmarshal(entry.Value(), &failure); err != nil {
		return fmt.Errorf("failure tracker failed to unmarshal entry: %w", err)
	}

	var errorPayload = struct {
		Id              string `json:"id"`
		ProducerID      string `json:"producer"`
		OrchestrationID string `json:"orchestration"`
		Error           string `json:"error"`
	}{
		Id:              entry.ID(),
		ProducerID:      entry.ProducerID(),
		OrchestrationID: orchestrationID,
		Error:           failure.Failure,
	}

	reason, err := json.Marshal(errorPayload)
	if err != nil {
		return fmt.Errorf("failure tracker failed to marshal error payload: %w", err)
	}

	failed := f.LogManager.MarkOrchestrationFailed(orchestrationID, failure.Failure)

	if err := f.LogManager.FinalizeOrchestration(orchestrationID, failed, reason, nil, failure.SkipWebhook); err != nil {
		isWebHookErr := strings.Contains(err.Error(), "failed to trigger webhook")
		return f.LogManager.AppendTaskFailureToLog(
			orchestrationID,
			FailureTrackerID,
			FailureTrackerID,
			fmt.Errorf("%s:%w", failure.Failure, err).Error(),
			0,
			isWebHookErr,
		)
	}
	return nil
}
