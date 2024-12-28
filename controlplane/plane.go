/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	v "github.com/RussellLuo/validating/v3"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid/v4"
	"github.com/rs/zerolog"
)

func NewControlPlane(openAIKey string) *ControlPlane {
	plane := &ControlPlane{
		projects:           make(map[string]*Project),
		services:           make(map[string]map[string]*ServiceInfo),
		orchestrationStore: make(map[string]*Orchestration),
		logWorkers:         make(map[string]map[string]context.CancelFunc),
		openAIKey:          openAIKey,
	}
	return plane
}

func (p *ControlPlane) Initialise(ctx context.Context,
	logMgr *LogManager,
	wsManager *WebSocketManager,
	vCache *VectorCache,
	Logger zerolog.Logger) {
	p.LogManager = logMgr
	p.Logger = Logger
	p.WebSocketManager = wsManager
	p.VectorCache = vCache
	p.VectorCache.StartCleanup(ctx)
}

func (p *ControlPlane) RegisterOrUpdateService(service *ServiceInfo) error {
	p.servicesMu.Lock()
	defer p.servicesMu.Unlock()

	if errs := v.Validate(service.Validation()); len(errs) > 0 {
		err := fmt.Errorf("service validation error: %w", errs)
		p.Logger.Error().
			Err(err).
			Str("ProjectID", service.ProjectID).
			Str("ServiceName", service.Name).
			Msgf("validated service")

		return err
	}

	projectServices, exists := p.services[service.ProjectID]
	if !exists {
		p.Logger.Debug().
			Str("ProjectID", service.ProjectID).
			Str("ServiceName", service.Name).
			Msgf("Creating new project service")
		projectServices = make(map[string]*ServiceInfo)
		p.services[service.ProjectID] = projectServices
	}

	if len(strings.TrimSpace(service.ID)) == 0 {
		service.ID = p.GenerateServiceKey()
		service.Version = 1
		service.IdempotencyStore = NewIdempotencyStore(0)

		p.Logger.Debug().
			Str("ProjectID", service.ProjectID).
			Str("ServiceName", service.Name).
			Msgf("Generating new service ID")
	} else {
		existingService, exists := projectServices[service.ID]
		if !exists {
			return fmt.Errorf("service with key %s not found in project %s", service.ID, service.ProjectID)
		}
		service.ID = existingService.ID
		service.Version = existingService.Version + 1
		service.IdempotencyStore = existingService.IdempotencyStore

		p.Logger.Debug().
			Str("ProjectID", service.ProjectID).
			Str("ServiceID", service.ID).
			Str("ServiceName", service.Name).
			Int64("ServiceVersion", service.Version).
			Msgf("Updating existing service")
	}
	projectServices[service.ID] = service

	return nil
}

func (p *ControlPlane) GetServiceByID(serviceID string) (*ServiceInfo, error) {
	projectID, err := p.GetProjectIDForService(serviceID)
	if err != nil {
		p.Logger.Error().Err(err).Str("serviceID", serviceID).Msg("Failed to get project ID from control plane")
		return nil, err
	}

	service, err := p.GetService(projectID, serviceID)
	if err != nil {
		p.Logger.Error().Err(err).
			Str("projectID", projectID).
			Str("serviceID", serviceID).
			Msg("Failed to get service for project")
		return nil, err
	}

	return service, nil
}

func (p *ControlPlane) GetService(projectID string, serviceID string) (*ServiceInfo, error) {
	p.servicesMu.RLock()
	defer p.servicesMu.RUnlock()

	projectServices := p.services[projectID]
	svc, exists := projectServices[serviceID]
	if !exists {
		return nil, fmt.Errorf("service %s not found for project %s", serviceID, projectID)
	}
	return svc, nil
}

func (p *ControlPlane) GetServiceName(projectID string, serviceID string) (string, error) {
	service, err := p.GetService(projectID, serviceID)
	if err != nil {
		return "", err
	}
	return service.Name, nil
}

func (p *ControlPlane) GetProjectByApiKey(key string) (*Project, error) {
	for _, project := range p.projects {
		if project.APIKey == key || contains(project.AdditionalAPIKeys, key) {
			return project, nil
		}
	}
	return nil, fmt.Errorf("no project found with the given API key: %s", key)
}

func contains(entries []string, v string) bool {
	for _, e := range entries {
		if e == v {
			return true
		}
	}
	return false
}

func (p *ControlPlane) ServiceBelongsToProject(svcID, projectID string) bool {
	p.servicesMu.RLock()
	defer p.servicesMu.RUnlock()

	projectServices, exists := p.services[projectID]
	if !exists {
		return false
	}
	_, ok := projectServices[svcID]
	return ok
}

func (p *ControlPlane) OrchestrationBelongsToProject(orchestrationID, projectID string) bool {
	p.orchestrationStoreMu.RLock()
	defer p.orchestrationStoreMu.RUnlock()

	orchestration, exists := p.orchestrationStore[orchestrationID]
	if !exists {
		return false
	}

	return orchestration.ProjectID == projectID
}

func (p *ControlPlane) GenerateProjectKey() string {
	return fmt.Sprintf("p_%s", shortuuid.New())
}

func (p *ControlPlane) GenerateOrchestrationKey() string {
	return fmt.Sprintf("o_%s", shortuuid.New())
}

func (p *ControlPlane) GenerateAPIKey() string {
	key := fmt.Sprintf("%s-%s", uuid.New(), uuid.New())
	hexString := strings.ReplaceAll(key, "-", "")
	return fmt.Sprintf("sk-orra-v1-%s", hexString)
}

func (p *ControlPlane) GenerateServiceKey() string {
	return fmt.Sprintf("s_%s", shortuuid.New())
}

func (p *ControlPlane) GetProjectIDForService(serviceID string) (string, error) {
	p.servicesMu.RLock()
	defer p.servicesMu.RUnlock()

	for projectID, pServices := range p.services {
		for svcId := range pServices {
			if svcId == serviceID {
				return projectID, nil
			}
		}
	}
	return "", fmt.Errorf("no project found for service %s", serviceID)
}

func (o *Orchestration) GetHealthCheckGracePeriod() time.Duration {
	if o.HealthCheckGracePeriod == nil {
		return HealthCheckGracePeriod
	}
	return o.HealthCheckGracePeriod.Duration
}

func (o *Orchestration) GetTimeout() time.Duration {
	if o.Timeout == nil {
		return TaskTimeout
	}
	return o.Timeout.Duration
}

func (o *Orchestration) FailedBeforeDecomposition() bool {
	return o.Status == Failed && o.Plan == nil
}

func (o *Orchestration) Executable() bool {
	return o.Status != NotActionable && o.Status != Failed
}

func (s *SubTask) extractDependencies() DependencyKeySet {
	out := make(DependencyKeySet)
	for _, source := range s.Input {
		if dep := extractDependencyID(source); dep != "" {
			out[dep] = struct{}{}
		}
	}
	return out
}

// extractDependencyID extracts the task ID from a dependency reference
// Example: "$task0.param1" returns "task0"
func extractDependencyID(input any) string {
	switch val := input.(type) {
	case string:
		matches := DependencyPattern.FindStringSubmatch(val)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}
