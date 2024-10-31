package main

import (
	"encoding/json"
	"sort"
	"time"
)

type OrchestrationView struct {
	ID        string          `json:"id"`
	Action    string          `json:"action"`
	Status    Status          `json:"status"`
	Error     json.RawMessage `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

type OrchestrationListView struct {
	Pending       []OrchestrationView `json:"pending,omitempty"`
	Processing    []OrchestrationView `json:"processing,omitempty"`
	Completed     []OrchestrationView `json:"completed,omitempty"`
	Failed        []OrchestrationView `json:"failed,omitempty"`
	NotActionable []OrchestrationView `json:"notActionable,omitempty"`
}

func (p *ControlPlane) GetOrchestrationList(projectID string) OrchestrationListView {
	// Get orchestrations for this project
	orchestrations := p.getProjectOrchestrations(projectID)

	// Convert to view objects and group by status
	grouped := make(map[Status][]OrchestrationView)
	for _, o := range orchestrations {
		view := OrchestrationView{
			ID:        o.ID,
			Action:    o.Action.Content,
			Status:    o.Status,
			Error:     o.Error,
			Timestamp: o.Timestamp,
		}

		grouped[o.Status] = append(grouped[o.Status], view)
	}

	// Sort each group by timestamp (newest first)
	for status := range grouped {
		sort.Slice(grouped[status], func(i, j int) bool {
			return grouped[status][i].Timestamp.After(grouped[status][j].Timestamp)
		})
	}

	return OrchestrationListView{
		Pending:       grouped[Pending],
		Processing:    grouped[Processing],
		Completed:     grouped[Completed],
		Failed:        grouped[Failed],
		NotActionable: grouped[NotActionable],
	}
}

func (p *ControlPlane) getProjectOrchestrations(projectID string) []*Orchestration {
	p.orchestrationStoreMu.RLock()
	defer p.orchestrationStoreMu.RUnlock()

	var result []*Orchestration
	for _, o := range p.orchestrationStore {
		if o.ProjectID == projectID {
			result = append(result, o)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	p.Logger.Trace().Interface("orchestrations", result).Msg("orchestrations for lis view")
	return result
}
