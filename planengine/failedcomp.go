/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"fmt"
	"sort"
	"time"
)

// StoreFailedCompensation persists a failed compensation and keeps it in memory
func (p *PlanEngine) StoreFailedCompensation(comp *FailedCompensation) error {
	// Set initial values if not already set
	if comp.ID == "" {
		comp.ID = p.GenerateCompensationKey()
	}
	if comp.Timestamp.IsZero() {
		comp.Timestamp = time.Now().UTC()
	}

	// Persist to storage first
	if err := p.failedCompStorage.StoreFailedCompensation(comp); err != nil {
		return fmt.Errorf("failed to store failed compensation: %w", err)
	}

	// Then update in-memory state
	p.failedCompsMu.Lock()
	defer p.failedCompsMu.Unlock()

	projectComps, exists := p.failedCompensations[comp.ProjectID]
	if !exists {
		projectComps = make(map[string]*FailedCompensation)
		p.failedCompensations[comp.ProjectID] = projectComps
	}
	projectComps[comp.ID] = comp

	p.Logger.Debug().
		Str("ID", comp.ID).
		Str("ProjectID", comp.ProjectID).
		Str("OrchestrationID", comp.OrchestrationID).
		Str("TaskID", comp.TaskID).
		Str("ServiceID", comp.ServiceID).
		Str("ServiceName", comp.ServiceName).
		Str("Status", comp.Status.String()).
		Str("ResolutionState", comp.ResolutionState.String()).
		Int("AttemptsMade", comp.AttemptsMade).
		Int("MaxAttempts", comp.MaxAttempts).
		Msg("Stored failed compensation")

	return nil
}

// UpdateFailedCompensation updates an existing failed compensation
func (p *PlanEngine) UpdateFailedCompensation(comp *FailedCompensation) error {
	// Update storage first
	if err := p.failedCompStorage.UpdateFailedCompensation(comp); err != nil {
		return fmt.Errorf("failed to update failed compensation: %w", err)
	}

	// Then update in-memory state
	p.failedCompsMu.Lock()
	defer p.failedCompsMu.Unlock()

	projectComps, exists := p.failedCompensations[comp.ProjectID]
	if !exists {
		return fmt.Errorf("no compensations found for project %s", comp.ProjectID)
	}

	if _, exists := projectComps[comp.ID]; !exists {
		return fmt.Errorf("failed compensation %s not found for project %s", comp.ID, comp.ProjectID)
	}

	projectComps[comp.ID] = comp

	p.Logger.Debug().
		Str("ID", comp.ID).
		Str("ProjectID", comp.ProjectID).
		Str("Status", comp.Status.String()).
		Str("ResolutionState", comp.ResolutionState.String()).
		Msg("Updated failed compensation")

	return nil
}

// ResolveFailedCompensation marks a failed compensation as resolved
func (p *PlanEngine) ResolveFailedCompensation(id string, resolution string) error {
	// First load the compensation
	comp, err := p.GetFailedCompensation(id)
	if err != nil {
		return err
	}

	// Update its state
	comp.ResolutionState = ResolutionResolved
	comp.Resolution = resolution
	comp.ResolvedAt = time.Now().UTC()

	// Persist changes
	return p.UpdateFailedCompensation(comp)
}

// IgnoreFailedCompensation marks a failed compensation as ignored
func (p *PlanEngine) IgnoreFailedCompensation(id string, reason string) error {
	// First load the compensation
	comp, err := p.GetFailedCompensation(id)
	if err != nil {
		return err
	}

	// Update its state
	comp.ResolutionState = ResolutionIgnored
	comp.Resolution = reason
	comp.ResolvedAt = time.Now().UTC()

	// Persist changes
	return p.UpdateFailedCompensation(comp)
}

// GetFailedCompensation retrieves a failed compensation by ID
func (p *PlanEngine) GetFailedCompensation(id string) (*FailedCompensation, error) {
	// Try memory cache first
	p.failedCompsMu.RLock()
	for _, projectComps := range p.failedCompensations {
		if comp, exists := projectComps[id]; exists {
			p.failedCompsMu.RUnlock()
			return comp, nil
		}
	}
	p.failedCompsMu.RUnlock()

	// Fall back to storage
	return p.failedCompStorage.LoadFailedCompensation(id)
}

// ListProjectFailedCompensations returns all failed compensations for a project
func (p *PlanEngine) ListProjectFailedCompensations(projectID string) ([]*FailedCompensation, error) {
	// Collect from memory first (this is faster than going to storage)
	p.failedCompsMu.RLock()
	defer p.failedCompsMu.RUnlock()

	projectComps, exists := p.failedCompensations[projectID]
	if !exists {
		return nil, nil // No compensations for this project
	}

	comps := make([]*FailedCompensation, 0, len(projectComps))
	for _, comp := range projectComps {
		comps = append(comps, comp)
	}

	// Sort by timestamp (newest first)
	sort.Slice(comps, func(i, j int) bool {
		return comps[i].Timestamp.After(comps[j].Timestamp)
	})

	return comps, nil
}

// ListOrchestrationFailedCompensations returns all failed compensations for an orchestration
func (p *PlanEngine) ListOrchestrationFailedCompensations(orchestrationID string) ([]*FailedCompensation, error) {
	// We could check memory first, but it's organized by project, not orchestration
	// So we'll delegate to storage which can query by orchestrationID more efficiently
	return p.failedCompStorage.ListOrchestrationFailedCompensations(orchestrationID)
}
