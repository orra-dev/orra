/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStorage(t *testing.T) (*BadgerLogStorage, func()) {
	// Create temp directory for test DB
	tmpDir, err := os.MkdirTemp("", "badger-test-*")
	require.NoError(t, err)

	// Setup logger
	logger := zerolog.New(zerolog.NewTestWriter(t))

	// Create storage
	storage, err := NewBadgerLogStorage(tmpDir, logger)
	require.NoError(t, err)

	// Return cleanup function
	cleanup := func() {
		err := storage.Close()
		assert.NoError(t, err)
		err = os.RemoveAll(tmpDir)
		assert.NoError(t, err)
	}

	return storage, cleanup
}

func TestBadgerLogStorage_StoreAndLoadEntries(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	orchestrationID := "test-orch-1"

	// Create test entries
	entries := []LogEntry{
		{
			EntryType:  "testing",
			Id:         "entry1",
			Value:      json.RawMessage(`{"key":"value1"}`),
			ProducerID: "producer1",
			AttemptNum: 1,
			Timestamp:  time.Now().UTC(),
		},
		{
			EntryType:  "testing",
			Id:         "entry2",
			Value:      json.RawMessage(`{"key":"value2"}`),
			ProducerID: "producer2",
			AttemptNum: 2,
			Timestamp:  time.Now().UTC(),
		},
	}

	// Set offsets manually for test
	entries[0].Offset = 1
	entries[1].Offset = 2

	// Store entries
	for _, entry := range entries {
		err := storage.Store(orchestrationID, entry)
		require.NoError(t, err)
	}

	// Load and verify entries
	loadedEntries, err := storage.LoadEntries(orchestrationID)
	require.NoError(t, err)
	assert.Len(t, loadedEntries, len(entries))

	// Verify entries are loaded in order
	for i, entry := range entries {
		assert.Equal(t, entry.GetOffset(), loadedEntries[i].GetOffset())
		//assert.Equal(t, entry.GetEntryType(), loadedEntries[i].GetEntryType())
		assert.Equal(t, entry.GetID(), loadedEntries[i].GetID())
		assert.Equal(t, entry.GetProducerID(), loadedEntries[i].GetProducerID())
		assert.Equal(t, entry.GetAttemptNum(), loadedEntries[i].GetAttemptNum())
		assert.JSONEq(t, string(entry.GetValue()), string(loadedEntries[i].GetValue()))
	}
}

func TestBadgerLogStorage_StoreAndLoadState(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create test state
	state := &OrchestrationState{
		ID:            "test-orch-1",
		ProjectID:     "test-project",
		Status:        Processing,
		CreatedAt:     time.Now().UTC(),
		LastUpdated:   time.Now().UTC(),
		TasksStatuses: map[string]Status{"task1": Completed},
	}

	// Store state
	err := storage.StoreState(state)
	require.NoError(t, err)

	// Load and verify state
	loadedState, err := storage.LoadState(state.ID)
	require.NoError(t, err)
	assert.Equal(t, state.ID, loadedState.ID)
	assert.Equal(t, state.ProjectID, loadedState.ProjectID)
	assert.Equal(t, state.Status, loadedState.Status)
	assert.Equal(t, state.TasksStatuses, loadedState.TasksStatuses)
}

func TestBadgerLogStorage_ListOrchestrationStates(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create test states
	states := []*OrchestrationState{
		{
			ID:            "test-orch-1",
			ProjectID:     "test-project",
			Status:        Processing,
			CreatedAt:     time.Now().UTC(),
			LastUpdated:   time.Now().UTC(),
			TasksStatuses: map[string]Status{"task1": Completed},
		},
		{
			ID:            "test-orch-2",
			ProjectID:     "test-project",
			Status:        Completed,
			CreatedAt:     time.Now().UTC(),
			LastUpdated:   time.Now().UTC(),
			TasksStatuses: map[string]Status{"task1": Completed},
		},
	}

	// Store states
	for _, state := range states {
		err := storage.StoreState(state)
		require.NoError(t, err)
	}

	// List and verify states
	loadedStates, err := storage.ListOrchestrationStates()
	require.NoError(t, err)
	assert.Len(t, loadedStates, len(states))

	// Create maps for easier comparison
	expectedMap := make(map[string]*OrchestrationState)
	actualMap := make(map[string]*OrchestrationState)

	for _, state := range states {
		expectedMap[state.ID] = state
	}
	for _, state := range loadedStates {
		actualMap[state.ID] = state
	}

	// Compare states
	for id, expected := range expectedMap {
		actual, exists := actualMap[id]
		assert.True(t, exists)
		assert.Equal(t, expected.ID, actual.ID)
		assert.Equal(t, expected.ProjectID, actual.ProjectID)
		assert.Equal(t, expected.Status, actual.Status)
		assert.Equal(t, expected.TasksStatuses, actual.TasksStatuses)
	}
}

func TestBadgerLogStorage_NonExistentState(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	// Try to load non-existent state
	_, err := storage.LoadState("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "orchestration state not found")
}
