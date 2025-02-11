/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	v "github.com/RussellLuo/validating/v3"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid/v4"
	"github.com/rs/zerolog"
)

type ValidationError struct {
	field string
	err   error
}

func (e ValidationError) Field() string {
	return e.field
}
func (e ValidationError) Error() string {
	return fmt.Sprintf("%v", e.err)
}

type SpecVersionError struct {
	err error
}

func (e SpecVersionError) Error() string {
	return fmt.Sprintf("%v", e.err)
}

func NewControlPlane() *ControlPlane {
	plane := &ControlPlane{
		projects:           make(map[string]*Project),
		services:           make(map[string]map[string]*ServiceInfo),
		orchestrationStore: make(map[string]*Orchestration),
		logWorkers:         make(map[string]map[string]context.CancelFunc),
		groundings:         make(map[string]map[string]*GroundingSpec),
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
	if p.VectorCache != nil {
		p.VectorCache.StartCleanup(ctx)
	}
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

// ApplyGroundingSpec adds domain grounding to a project after validation
func (p *ControlPlane) ApplyGroundingSpec(projectID string, spec *GroundingSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}

	p.groundingsMu.Lock()
	defer p.groundingsMu.Unlock()

	if p.groundings == nil {
		p.groundings = make(map[string]map[string]*GroundingSpec)
	}

	if p.groundings[projectID] == nil {
		p.groundings[projectID] = make(map[string]*GroundingSpec)
	}

	if existing, ok := p.groundings[projectID][spec.Name]; ok && existing.Version == spec.Version {
		return SpecVersionError{err: fmt.Errorf(
			"project %s already has a grounding spec with name %s and version %s",
			projectID,
			spec.Name,
			spec.Version,
		)}
	}

	// Store the spec
	p.groundings[projectID][spec.Name] = spec

	p.Logger.Debug().
		Str("projectID", projectID).
		Str("name", spec.Name).
		Str("domain", spec.Domain).
		Msgf("Added grounding spec with %d action uses cases", len(spec.UseCases))

	return nil
}

// GetGroundingSpecs retrieves all domain groundings for a project
func (p *ControlPlane) GetGroundingSpecs(projectID string) []GroundingSpec {
	p.groundingsMu.RLock()
	defer p.groundingsMu.RUnlock()

	p.Logger.Trace().
		Str("projectID", projectID).
		Msg("Getting grounding specs")

	if p.groundings == nil {
		p.Logger.Trace().
			Str("projectID", projectID).
			Msg("No grounding specs found")

		return nil
	}

	groundings, exists := p.groundings[projectID]
	if !exists {
		return nil
	}

	// Convert map to slice
	out := make([]GroundingSpec, 0, len(groundings))
	for _, spec := range groundings {
		out = append(out, *spec)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out
}

// RemoveGroundingSpecByName removes a specific domain grounding from a project by its name
func (p *ControlPlane) RemoveGroundingSpecByName(projectID string, name string) error {
	p.groundingsMu.Lock()
	defer p.groundingsMu.Unlock()

	if p.groundings == nil {
		return fmt.Errorf("grounding spec not found")
	}

	projectExamples, exists := p.groundings[projectID]
	if !exists {
		return fmt.Errorf("grounding spec not found")
	}

	delete(projectExamples, name)

	// If project has no more examples, remove the project entry
	if len(projectExamples) == 0 {
		delete(p.groundings, projectID)
	}

	p.Logger.Debug().
		Str("projectID", projectID).
		Str("name", name).
		Msg("Removed grounding spec")

	return nil
}

// RemoveProjectGrounding removes all domain grounding for a project
func (p *ControlPlane) RemoveProjectGrounding(projectID string) error {
	p.groundingsMu.Lock()
	defer p.groundingsMu.Unlock()

	if p.groundings == nil {
		return nil
	}

	delete(p.groundings, projectID)

	p.Logger.Debug().
		Str("projectID", projectID).
		Msg("Removed all domain examples")

	return nil
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

func (s *SubTask) extractDependencies() TaskDependenciesWithKeys {
	out := make(TaskDependenciesWithKeys)
	for taskKey, source := range s.Input {
		dep, key := extractDependencyIDAndKey(source)
		if dep == "" {
			continue
		}
		if _, ok := out[dep]; !ok {
			out[dep] = []TaskDependencyMapping{{taskKey, key}}
		} else {
			out[dep] = append(out[dep], TaskDependencyMapping{taskKey, key})
		}
	}
	return out
}

// extractDependencyIDAndKey extracts the task ID and task dependency key from a dependency reference
// Example: "$task0.param1" returns "task0", "param1"
func extractDependencyIDAndKey(input any) (depID string, depKey string) {
	switch val := input.(type) {
	case string:
		matches := DependencyPattern.FindStringSubmatch(val)
		if len(matches) <= 1 {
			return "", ""
		}
		depID := matches[1]
		return depID, strings.TrimPrefix(val, fmt.Sprintf("$%s.", depID))
	}
	return "", ""
}
