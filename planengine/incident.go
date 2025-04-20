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

func NewIncidentTracker(logManager *LogManager) LogWorker {
	return &IncidentTracker{
		LogManager: logManager,
		logState: &LogState{
			LastOffset: 0,
			Processed:  make(map[string]bool),
		},
	}
}

func (t *IncidentTracker) Start(ctx context.Context, orchestrationID string) {
	logStream := t.LogManager.GetLog(orchestrationID)
	if logStream == nil {
		t.LogManager.Logger.Error().Str("orchestrationID", orchestrationID).Msg("Log stream not found for orchestration")
		return
	}

	// Channel to receive new log entries
	entriesChan := make(chan LogEntry, 100)

	// Start a goroutine for continuous polling
	go t.PollLog(ctx, orchestrationID, logStream, entriesChan)

	// Process entries as they come in
	for {
		select {
		case entry := <-entriesChan:
			if err := t.processEntry(entry, orchestrationID); err != nil {
				t.LogManager.Logger.
					Error().
					Err(err).
					Str("orchestrationID", orchestrationID).
					Interface("entry", entry).
					Msg("Failure tracker failed to process entry for orchestration.")
				return
			}
		case <-ctx.Done():
			t.LogManager.Logger.Info().Msgf("Failure tracker in orchestration %s is stopping", orchestrationID)
			return
		}
	}
}

func (t *IncidentTracker) PollLog(ctx context.Context, _ string, logStream *Log, entriesChan chan<- LogEntry) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:

			entries := logStream.ReadFrom(t.logState.LastOffset)
			for _, entry := range entries {
				if !t.shouldProcess(entry) {
					continue
				}

				select {
				case entriesChan <- entry:
					t.logState.LastOffset = entry.GetOffset() + 1
				case <-ctx.Done():
					return
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

func (t *IncidentTracker) shouldProcess(entry LogEntry) bool {
	return entry.GetEntryType() == "task_failure" || entry.GetEntryType() == "task_aborted_output"
}

func (t *IncidentTracker) processEntry(entry LogEntry, orchestrationID string) error {
	// Mark this entry as processed
	t.logState.Processed[entry.GetID()] = true

	switch entry.GetEntryType() {
	case "task_aborted_output":
		aborted := t.LogManager.MarkOrchestrationAborted(orchestrationID)
		if err := t.LogManager.FinalizeOrchestration(orchestrationID, aborted, nil, nil, entry.GetValue(), entry.GetTimestamp(), false); err != nil {
			isWebHookErr := strings.Contains(err.Error(), "failed to trigger webhook")
			return t.LogManager.AppendTaskFailureToLog(
				orchestrationID,
				IncidentTrackerID,
				IncidentTrackerID,
				err.Error(),
				0,
				isWebHookErr,
			)
		}

	default:
		var failure LoggedFailure
		if err := json.Unmarshal(entry.GetValue(), &failure); err != nil {
			return fmt.Errorf("failure tracker failed to unmarshal entry: %w", err)
		}

		var errorPayload = struct {
			Id         string `json:"id"`
			ProducerID string `json:"producer"`
			Error      string `json:"error"`
		}{
			Id:         entry.GetID(),
			ProducerID: entry.GetProducerID(),
			Error:      failure.Failure,
		}

		reason, err := json.Marshal(errorPayload)
		if err != nil {
			return fmt.Errorf("failure tracker failed to marshal error payload: %w", err)
		}

		failed := t.LogManager.MarkOrchestrationFailed(orchestrationID, failure.Failure)

		if err := t.LogManager.FinalizeOrchestration(orchestrationID, failed, reason, nil, nil, entry.GetTimestamp(), failure.SkipWebhook); err != nil {
			isWebHookErr := strings.Contains(err.Error(), "failed to trigger webhook")
			return t.LogManager.AppendTaskFailureToLog(
				orchestrationID,
				IncidentTrackerID,
				IncidentTrackerID,
				fmt.Errorf("%s:%w", failure.Failure, err).Error(),
				0,
				isWebHookErr,
			)
		}
	}

	return nil
}
