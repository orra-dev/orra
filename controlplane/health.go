package main

import (
	"time"

	"github.com/rs/zerolog"
)

func NewHealthCoordinator(plane *ControlPlane, manager *LogManager, logger zerolog.Logger) *HealthCoordinator {
	return &HealthCoordinator{
		plane:             plane,
		logManager:        manager,
		logger:            logger,
		lastHealthState:   make(map[string]bool),
		lastHealthRestart: make(map[string]time.Time),
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

	//if updateTime, exists := h.lastHealthRestart[serviceID]; exists && time.Since(updateTime) < 30*time.Second {
	//	return
	//}

	h.logger.Debug().
		Str("ProjectID", service.ProjectID).
		Str("serviceID", service.ID).
		Str("serviceName", service.Name).
		Bool("NewHealth", isHealthy).
		Msg("Health change detected")

	h.lastHealthState[serviceID] = isHealthy

	if !isHealthy {
		return
	}

	//orchestrationsAndTasks := h.GetActiveOrchestrationsAndTasksForService(service)
	//h.restartOrchestrationTasks(orchestrationsAndTasks)
	//h.restartOrchestrationTasks(service)
	//h.lastHealthRestart[serviceID] = time.Now().UTC()
}

//func (h *HealthCoordinator) GetActiveOrchestrationsAndTasksForService(service *ServiceInfo) map[string]map[string]SubTask {
//	h.mu.RLock()
//	defer h.mu.RUnlock()
//
//	return h.logManager.GetActiveOrchestrationsWithTasks(service)
//}

//func (h *HealthCoordinator) restartOrchestrationTasks(orchestrationsAndTasks map[string]map[string]SubTask) {
//	h.mu.Lock()
//	defer h.mu.Unlock()
//
//	for orchestrationID, tasks := range orchestrationsAndTasks {
//		for _, task := range tasks {
//			completed, err := h.logManager.IsTaskCompleted(orchestrationID, task.ID)
//			if err != nil {
//				h.logger.Error().
//					Err(err).
//					Str("orchestrationID", orchestrationID).
//					Str("taskID", task.ID).
//					Msg("failed to check if task is completed during restart - continuing")
//			}
//
//			if !completed {
//				h.restartTask(orchestrationID, task)
//			}
//		}
//	}
//}

func (h *HealthCoordinator) restartOrchestrationTasks(service *ServiceInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()

	orchestrationsWithTasks := h.logManager.GetActiveOrchestrationsWithTasks(service.ProjectID, service.ID)
	h.logger.Debug().Interface("orchestrationsWithTasks", orchestrationsWithTasks).Msg("tasks for restarting")

	for orchestrationID, tasks := range orchestrationsWithTasks {
		for _, task := range tasks {
			completed, err := h.logManager.IsTaskCompleted(orchestrationID, task.ID)
			if err != nil {
				h.logger.Error().
					Err(err).
					Str("orchestrationID", orchestrationID).
					Str("taskID", task.ID).
					Msg("failed to check if task is completed during restart - continuing")
				continue
			}

			if !completed {
				h.restartTask(orchestrationID, task)
			}
		}
	}
}

func (h *HealthCoordinator) restartTask(orchestrationID string, task SubTask) {
	h.logger.Debug().
		Str("orchestrationID", orchestrationID).
		Str("taskID", task.ID).
		Msg("Restarting task")

	h.plane.StopTaskWorker(orchestrationID, task.ID)
	h.plane.CreateAndStartTaskWorker(orchestrationID, task)
}
