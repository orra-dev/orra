package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

func NewHealthCoordinator(plane *ControlPlane, manager *LogManager, logger zerolog.Logger) *HealthCoordinator {
	return &HealthCoordinator{
		plane:           plane,
		logManager:      manager,
		logger:          logger,
		lastHealthState: make(map[string]bool),
		pauseTimers:     make(map[string]*time.Timer),
	}
}

func (h *HealthCoordinator) handleServiceHealthChange(serviceID string, isHealthy bool) {
	service, err := h.plane.GetServiceByID(serviceID)
	if err != nil {
		h.logger.Error().Err(err).Str("service", serviceID).Msg("Error getting service")
		return
	}

	if health, exists := h.lastHealthState[serviceID]; exists && health == isHealthy {
		return
	}

	h.lastHealthState[serviceID] = isHealthy
	orchestrationsAndTasks := h.GetActiveOrchestrationsAndTasksForService(service)
	h.logger.Debug().
		Interface("orchestrationsAndTasks", orchestrationsAndTasks).
		Msg("active orchestrations and tasks for service")

	if !isHealthy {
		// Mark orchestrations as paused, stop workers
		h.logManager.UpdateActiveOrchestrations(
			orchestrationsAndTasks,
			serviceID,
			"service_unhealthy",
			Processing,
			Paused,
		)
		h.stopTasks(orchestrationsAndTasks)

		// Start timeout timers for each paused orchestration
		for orchestrationID := range orchestrationsAndTasks {
			h.startPauseTimeout(orchestrationID, serviceID)
		}
		return
	}

	// Service is healthy again - restart tasks with clean state
	h.logManager.UpdateActiveOrchestrations(
		orchestrationsAndTasks,
		serviceID,
		"service_healthy",
		Paused,
		Processing,
	)

	// Cancel timeout timers
	for orchestrationID := range orchestrationsAndTasks {
		h.cancelPauseTimeout(orchestrationID)
	}

	h.restartTasks(orchestrationsAndTasks)
}

func (h *HealthCoordinator) GetActiveOrchestrationsAndTasksForService(service *ServiceInfo) map[string]map[string]SubTask {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.logManager.GetActiveOrchestrationsWithTasks(service.ProjectID, service.ID)
}

func (h *HealthCoordinator) stopTasks(orchestrationsAndTasks map[string]map[string]SubTask) {
	for orchestrationID, tasks := range orchestrationsAndTasks {
		for _, task := range tasks {
			completed := h.logManager.IsTaskCompleted(orchestrationID, task.ID)
			if completed {
				continue
			}
			h.logger.Debug().
				Str("orchestrationID", orchestrationID).
				Str("taskID", task.ID).
				Msg("Stopping task")

			h.plane.StopTaskWorker(orchestrationID, task.ID)
		}
	}
}

func (h *HealthCoordinator) restartTasks(orchestrationsAndTasks map[string]map[string]SubTask) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for orchestrationID, tasks := range orchestrationsAndTasks {
		for _, task := range tasks {
			completed := h.logManager.IsTaskCompleted(orchestrationID, task.ID)
			if completed {
				continue
			}
			h.logger.Debug().
				Str("orchestrationID", orchestrationID).
				Str("taskID", task.ID).
				Msg("Restarting task")

			h.plane.CreateAndStartTaskWorker(orchestrationID, task)
		}
	}
}

func (h *HealthCoordinator) startPauseTimeout(orchestrationID, serviceID string) {
	h.pauseTimersMu.Lock()
	defer h.pauseTimersMu.Unlock()

	// Cancel existing timer if any
	if timer, exists := h.pauseTimers[orchestrationID]; exists {
		timer.Stop()
	}

	// Start new timeout timer
	h.pauseTimers[orchestrationID] = time.AfterFunc(MaxServiceDowntime, func() {
		h.handlePauseTimeout(orchestrationID, serviceID)
	})
}

func (h *HealthCoordinator) cancelPauseTimeout(orchestrationID string) {
	h.pauseTimersMu.Lock()
	defer h.pauseTimersMu.Unlock()

	if timer, exists := h.pauseTimers[orchestrationID]; exists {
		timer.Stop()
		delete(h.pauseTimers, orchestrationID)
	}
}

func (h *HealthCoordinator) handlePauseTimeout(orchestrationID, serviceID string) {
	h.pauseTimersMu.Lock()
	delete(h.pauseTimers, orchestrationID)
	h.pauseTimersMu.Unlock()

	h.logger.Error().
		Str("orchestrationID", orchestrationID).
		Str("serviceID", serviceID).
		Dur("timeout", MaxServiceDowntime).
		Msg("Service remained unhealthy beyond maximum pause duration")

	reason := fmt.Sprintf("Service %s remained unhealthy beyond maximum duration of %v",
		serviceID, MaxServiceDowntime)

	// Mark orchestration as failed
	marshaledReason, _ := json.Marshal(reason)
	if err := h.logManager.FinalizeOrchestration(
		orchestrationID,
		Failed,
		marshaledReason,
		nil,
	); err != nil {
		h.logger.Error().
			Err(err).
			Str("orchestrationID", orchestrationID).
			Msg("Failed to finalize timed out orchestration")
	}
}

// Shutdown Clean up on shutdown
func (h *HealthCoordinator) Shutdown() {
	h.pauseTimersMu.Lock()
	defer h.pauseTimersMu.Unlock()

	for _, timer := range h.pauseTimers {
		timer.Stop()
	}
	h.pauseTimers = make(map[string]*time.Timer)
}
