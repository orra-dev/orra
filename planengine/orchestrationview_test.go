/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestState helps track and build test state
type TestState struct {
	plane           *PlanEngine
	orchestrationID string
	baseTime        time.Time
	attempts        int
}

func newTestSetup() *TestState {
	return &TestState{
		baseTime: time.Now().UTC(),
		attempts: 0,
	}
}

// Helper to create test project and service
func (ts *TestState) setupBase() func() {
	logger := zerolog.New(zerolog.NewTestWriter(&testing.T{}))
	tmpDir, _ := os.MkdirTemp("", "badger-test-*")
	db, _ := NewBadgerDB(tmpDir, logger)

	dbCleanup := func() {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
	}
	// Create plan engine
	ts.plane = NewPlanEngine()

	// Register project
	ts.plane.projects["p_test"] = &Project{
		ID:     "p_test",
		APIKey: "test-key",
	}

	// Register service
	ts.plane.services["p_test"] = map[string]*ServiceInfo{
		"s_echo": {
			ID:          "s_echo",
			Name:        "Echo Service",
			Type:        Service,
			Description: "Test service that echoes input",
			ProjectID:   "p_test",
		},
	}

	// Create base orchestration
	o := &Orchestration{
		ID:        "o_test",
		ProjectID: "p_test",
		Action: Action{
			Content: "Echo a message",
		},
		Status:    Processing,
		Timestamp: ts.baseTime,
		TaskZero:  json.RawMessage(`{"message": "Hello World", "userId": "user123"}`),
		Plan: &ExecutionPlan{
			Tasks: []*SubTask{
				{
					ID:      "task1",
					Service: "s_echo",
					Input: map[string]interface{}{
						"text": "$task0.message",
						"user": "$task0.userId",
					},
				},
			},
		},
	}
	ts.orchestrationID = o.ID

	// Setup log manager
	ts.plane.LogManager, _ = NewLogManager(context.Background(), db, time.Hour, ts.plane)
	ts.plane.LogManager.PrepLogForOrchestration(o.ProjectID, o.ID, o.Plan)
	ts.plane.orchestrationStorage = db

	// Initialize orchestration state
	ts.plane.LogManager.orchestrations[o.ID] = &OrchestrationState{
		ID:            o.ID,
		ProjectID:     o.ProjectID,
		Plan:          o.Plan,
		TasksStatuses: make(map[string]Status),
		CreatedAt:     ts.baseTime,
		LastUpdated:   ts.baseTime,
	}

	ts.plane.orchestrationStore[o.ID] = o

	return dbCleanup
}

// Helper to add a task state transition
func (ts *TestState) addTaskState(status Status, errMsg string, afterMinutes int) {
	timestamp := ts.baseTime.Add(time.Duration(afterMinutes) * time.Minute)
	err := error(nil)
	if errMsg != "" {
		err = errors.New(errMsg)
	}

	_ = ts.plane.LogManager.AppendTaskStatusEvent(ts.orchestrationID, "task1", "s_echo", status, err, timestamp, ts.attempts)

	if status == Failed {
		ts.plane.LogManager.orchestrations[ts.orchestrationID].Status = Failed
		ts.plane.LogManager.orchestrations[ts.orchestrationID].Error = errMsg
		ts.attempts++
	}
}

// Helper to add task output
func (ts *TestState) addTaskOutput(output string) {
	ts.plane.LogManager.AppendToLog(ts.orchestrationID, "task_output", "task1", json.RawMessage(output), "s_echo", ts.attempts)
}

func TestInspectOrchestration(t *testing.T) {
	t.Run("successful inspection with task0 resolution", func(t *testing.T) {
		ts := newTestSetup()
		cleanDB := ts.setupBase()
		defer cleanDB()

		// Add task transitions
		ts.addTaskState(Processing, "", 1)
		ts.addTaskOutput(`{"result":"Hello World"}`)
		ts.addTaskState(Completed, "", 10)

		// Verify inspection
		resp, err := ts.plane.InspectOrchestration(ts.orchestrationID)
		require.NoError(t, err)

		// Verify orchestration details
		assert.Equal(t, ts.orchestrationID, resp.ID)
		assert.Equal(t, "Echo a message", resp.Action)
		assert.Equal(t, Processing, resp.Status)

		// Verify task details
		require.Len(t, resp.Tasks, 1)
		task := resp.Tasks[0]
		assert.Equal(t, "task1", task.ID)
		assert.Equal(t, "s_echo", task.ServiceID)
		assert.Equal(t, "Echo Service", task.ServiceName)
		assert.Equal(t, Completed, task.Status)

		// Verify status transitions
		require.Len(t, task.StatusHistory, 2)
		assert.Equal(t, Processing, task.StatusHistory[0].Status)
		assert.Equal(t, Completed, task.StatusHistory[1].Status)

		// Verify resolved task0 references
		var input map[string]interface{}
		require.NoError(t, json.Unmarshal(task.Input, &input))
		assert.Equal(t, "Hello World", input["text"])
		assert.Equal(t, "user123", input["user"])

		// Verify output
		var output map[string]interface{}
		require.NoError(t, json.Unmarshal(task.Output, &output))
		assert.Equal(t, "Hello World", output["result"])
	})

	t.Run("inspect orchestration with paused task", func(t *testing.T) {
		ts := newTestSetup()
		cleanDB := ts.setupBase()
		defer cleanDB()

		// Add task transitions including pause
		ts.addTaskState(Processing, "", 1)
		ts.addTaskState(Paused, "service unhealthy", 2)
		ts.addTaskState(Processing, "", 5)
		ts.addTaskOutput(`{"result":"Hello World"}`)
		ts.addTaskState(Completed, "", 10)

		resp, err := ts.plane.InspectOrchestration(ts.orchestrationID)
		require.NoError(t, err)

		task := resp.Tasks[0]
		assert.Equal(t, Completed, task.Status)

		// Verify full status history
		require.Len(t, task.StatusHistory, 4)
		assert.Equal(t, Processing, task.StatusHistory[0].Status)
		assert.Equal(t, Paused, task.StatusHistory[1].Status)
		assert.Contains(t, task.StatusHistory[1].Error, "service unhealthy")
		assert.Equal(t, Processing, task.StatusHistory[2].Status)
		assert.Equal(t, Completed, task.StatusHistory[3].Status)
	})

	t.Run("inspect failed orchestration", func(t *testing.T) {
		ts := newTestSetup()
		cleanDB := ts.setupBase()
		defer cleanDB()

		// Add task transitions ending in failure
		ts.addTaskState(Processing, "", 1)
		ts.addTaskState(Failed, "validation failed", 2)

		resp, err := ts.plane.InspectOrchestration(ts.orchestrationID)
		require.NoError(t, err)

		task := resp.Tasks[0]
		assert.Equal(t, Failed, task.Status)

		// Verify status history includes failure
		lastStatus := task.StatusHistory[len(task.StatusHistory)-1]
		assert.Equal(t, Failed, lastStatus.Status)
		assert.Contains(t, lastStatus.Error, "validation failed")
	})

	t.Run("inspect non-existent orchestration", func(t *testing.T) {
		ts := newTestSetup()
		cleanDB := ts.setupBase()
		defer cleanDB()

		_, err := ts.plane.InspectOrchestration("o_nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("inspect orchestration with missing task0 reference", func(t *testing.T) {
		ts := newTestSetup()
		cleanDB := ts.setupBase()
		defer cleanDB()

		// Add invalid task0 reference
		ts.plane.orchestrationStore[ts.orchestrationID].Plan.Tasks[0].Input["invalid"] = "$task0.nonexistent"

		_, err := ts.plane.InspectOrchestration(ts.orchestrationID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve task0 references")
	})
}

func TestInspectOrchestration_NotActionable(t *testing.T) {
	ts := newTestSetup()
	cleanDB := ts.setupBase()
	defer cleanDB()

	// Update the test orchestration to be NotActionable
	orchestration := ts.plane.orchestrationStore[ts.orchestrationID]
	orchestration.Status = NotActionable
	orchestration.Error = json.RawMessage(`"Cannot process order: No payment service available"`)
	// Add a "final" task that explains why it's not actionable
	orchestration.Plan = &ExecutionPlan{
		Tasks: []*SubTask{
			{
				ID: "final",
				Input: map[string]interface{}{
					"error": "Cannot process order: No payment service available",
				},
			},
		},
	}

	// Test inspection
	resp, err := ts.plane.InspectOrchestration(ts.orchestrationID)
	require.NoError(t, err)

	// Verify response
	assert.Equal(t, ts.orchestrationID, resp.ID)
	assert.Equal(t, NotActionable, resp.Status)
	assert.Equal(t, "Echo a message", resp.Action)
	assert.Equal(t, "\"Cannot process order: No payment service available\"", string(resp.Error))
	assert.Empty(t, resp.Tasks)
}

func TestInspectOrchestration_NotActionableWithoutTasks(t *testing.T) {
	ts := newTestSetup()
	cleanDB := ts.setupBase()
	defer cleanDB()

	// Create NotActionable orchestration without any tasks
	orchestration := ts.plane.orchestrationStore[ts.orchestrationID]
	orchestration.Status = NotActionable
	orchestration.Error = json.RawMessage(`"Invalid action: unsupported operation"`)
	orchestration.Plan = &ExecutionPlan{Tasks: []*SubTask{}}

	resp, err := ts.plane.InspectOrchestration(ts.orchestrationID)
	require.NoError(t, err)

	assert.Equal(t, NotActionable, resp.Status)
	assert.Equal(t, "\"Invalid action: unsupported operation\"", string(resp.Error))
	assert.Empty(t, resp.Tasks)
}
