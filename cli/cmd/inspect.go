/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/ezodude/orra/cli/internal/api"
	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

func newInspectCmd(opts *CliOpts) *cobra.Command {
	var detailed bool
	var shortUpdates bool
	var longUpdates bool

	cmd := &cobra.Command{
		Use:   "inspect [orchestration-id]",
		Short: `Return orchestrated action details`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, projectName, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			client := opts.ApiClient.
				SetBaseUrl(proj.ServerAddr).
				SetApiKey(proj.CliAuth)

			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			orchestrationID := args[0]
			inspection, err := client.GetOrchestrationInspection(ctx, orchestrationID)
			if err != nil {
				return fmt.Errorf("failed to inspect orchestration - %w", err)
			}

			// Project Info Section
			fmt.Printf("Project: %s\nServer:  %s\n", projectName, proj.ServerAddr)

			// Overview Section
			fmt.Printf("\n┌─ Orchestration Details %s\n", getStatusSuffix(inspection.Status))
			fmt.Printf("│ ID:      %s\n", inspection.ID)
			fmt.Printf("│ Status:  %s\n", formatStatus(inspection.Status.String()))
			fmt.Printf("│ Action:  %s\n", inspection.Action)
			fmt.Printf("│ Created: %s ago\n", formatDuration(inspection.Duration))
			if inspection.Error != nil {
				fmt.Printf("│ Error:   %s\n", string(inspection.Error))
			}
			fmt.Printf("└─────\n")

			// Add Abort Information section if this is an aborted orchestration
			if inspection.Status == "aborted" {
				fmt.Printf("\n┌─ Abort Information\n")

				if inspection.AbortInfo != nil {
					fmt.Printf("│ Task:      %s\n", inspection.AbortInfo.TaskID)
					fmt.Printf("│ Service:   %s\n", inspection.AbortInfo.ServiceName)

					// Format all fields in the abort payload
					if inspection.AbortInfo.Payload != nil {
						fields := formatAbortPayload(inspection.AbortInfo.Payload)
						if len(fields) > 0 {
							for _, field := range fields {
								// Format field name with first letter capitalized
								fmt.Printf("│ %-10s %s\n", capitalizeFirst(field.Key)+":", field.Value)
							}
						} else {
							fmt.Printf("│ Payload:   %s\n", string(inspection.AbortInfo.Payload))
						}
					}
				} else {
					// Handle case where orchestration was aborted but no AbortInfo is available
					// Find the aborted task
					var abortedTask *api.TaskInspectResponse
					for i := range inspection.Tasks {
						if inspection.Tasks[i].Status == "aborted" {
							abortedTask = &inspection.Tasks[i]
							break
						}
					}

					if abortedTask != nil {
						fmt.Printf("│ Task:      %s\n", abortedTask.ID)
						fmt.Printf("│ Service:   %s\n", abortedTask.ServiceName)
						fmt.Printf("│ Details:   No additional information available\n")
					} else {
						fmt.Printf("│ Details:   No information available about the abort\n")
					}
				}
				fmt.Printf("└─────\n")
			}

			// Tasks Table
			if len(inspection.Tasks) > 0 {
				fmt.Printf("\n┌─ Tasks\n")
				// Header
				fmt.Printf("│ %-8s %-30s %-14s %-20s %-10s %s\n",
					"ID", "SERVICE", "STATUS", "COMPENSATION", "DURATION", "LAST ERROR")
				fmt.Printf("│ %s\n", strings.Repeat("─", 100))

				// Task rows
				for _, task := range inspection.Tasks {
					fmt.Printf("│ %-8s %-30s %-14s %-20s %-10s %s\n",
						task.ID,
						task.ServiceName,
						formatStatus(task.Status.String()),
						formatCompensation(task.Compensation.String()),
						formatDuration(task.Duration),
						formatInspectionError(task.Error),
					)
				}
				fmt.Printf("└─────\n")
			}

			// Detailed Status History
			if detailed && len(inspection.Tasks) > 0 {
				fmt.Printf("\n┌─ Task Execution Details\n")
				for i, task := range inspection.Tasks {
					fmt.Printf("│\n│ %s (%s)\n", task.ServiceName, task.ID)
					fmt.Printf("│ %s\n", strings.Repeat("─", 50))

					// Status History
					for _, status := range task.StatusHistory {
						timestamp := status.Timestamp.Format("15:04:05")
						statusLine := fmt.Sprintf("│ %s  %s",
							timestamp,
							formatStatus(status.Status.String()),
						)
						if status.Error != "" {
							statusLine += fmt.Sprintf(" - %s", status.Error)
						}
						fmt.Println(statusLine)
					}

					if (shortUpdates || longUpdates) && len(task.InterimResults) > 0 {
						// Show the simplified timeline
						timelineStr := renderTaskTimeline(task.StatusHistory, task.InterimResults)
						if timelineStr != "" {
							fmt.Println(timelineStr)
						}

						fmt.Printf("│\n│ Progress Updates %s\n", strings.Repeat("─", 35))

						if longUpdates {
							// Show all updates
							for j, result := range task.InterimResults {
								fmt.Printf("│ %s  Update %d/%d\n",
									result.Timestamp.Format("15:04:05"),
									j+1,
									len(task.InterimResults))
								printJSONWithPrefix(result.Data, "│   ")
								fmt.Printf("│\n")
							}
						} else {
							// Show only first and last updates
							first := task.InterimResults[0]
							fmt.Printf("│ %s  First Update\n", first.Timestamp.Format("15:04:05"))
							printJSONWithPrefix(first.Data, "│   ")

							// Show last update if different from first
							if len(task.InterimResults) > 1 {
								last := task.InterimResults[len(task.InterimResults)-1]
								fmt.Printf("│\n│ %s  Latest Update\n", last.Timestamp.Format("15:04:05"))
								printJSONWithPrefix(last.Data, "│   ")
							}

							if len(task.InterimResults) > 2 {
								fmt.Printf("│\n│ Use --long-updates to show all %d updates\n",
									len(task.InterimResults))
							}
						}

						// Add divider after progress updates section
						fmt.Printf("│\n│ %s\n", strings.Repeat("─", 50))
					}

					// Print compensation history if present
					if len(task.CompensationHistory) > 0 {
						fmt.Printf("│ Compensating %s\n", strings.Repeat("─", 37))

						for _, comp := range task.CompensationHistory {
							timestamp := comp.Timestamp.Format("15:04:05")
							statusLine := fmt.Sprintf("│ %s  %s",
								timestamp,
								formatCompensation(comp.String()),
							)
							if comp.Error != "" {
								statusLine += fmt.Sprintf(" - %s", comp.Error)
							}
							fmt.Println(statusLine)
						}
					}

					// Input/Output with proper indentation
					if task.Input != nil {
						fmt.Printf("│\n│ Input:\n")
						printJSONWithPrefix(task.Input, "│   ")
					}
					if task.Output != nil {
						fmt.Printf("│\n│ Output:\n")
						printJSONWithPrefix(task.Output, "│   ")
					}

					// Display Abort Payload if task was aborted
					if task.Status == "aborted" && task.AbortPayload != nil {
						fmt.Printf("│\n│ Abort Payload:\n")
						printJSONWithPrefix(task.AbortPayload, "│   ")
					}

					// Add spacing between tasks
					if i < len(inspection.Tasks)-1 {
						fmt.Printf("│\n│ %s\n", strings.Repeat("· ", 25))
					}
				}
				fmt.Printf("└─────\n")

				// Final Results in its own section
				if len(inspection.Results) > 0 {
					fmt.Printf("\n┌─ Final Results\n")
					fmt.Printf("│\n")
					printJSONWithPrefix(inspection.Results[0], "│   ")
					fmt.Printf("└─────\n")
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&detailed, "detailed", "d", false, "Show detailed task history with I/O")
	cmd.Flags().BoolVarP(&shortUpdates, "updates", "u", false, "Show summarized progress updates (first and last only)")
	cmd.Flags().BoolVar(&longUpdates, "long-updates", false, "Show all progress updates with complete details")

	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if shortUpdates && longUpdates {
			return fmt.Errorf("--updates and --long-updates flags cannot be used together")
		}
		return nil
	}

	return cmd
}

func getStatusSuffix(status api.Status) string {
	switch strings.ToLower(status.String()) {
	case "failed":
		return "[FAILED]"
	case "not_actionable":
		return "[NOT ACTIONABLE]"
	case "aborted":
		return "[ABORTED]"
	default:
		return ""
	}
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// formatAbortPayload formats the abort payload for display in the CLI
// It extracts all fields and returns them sorted alphabetically
func formatAbortPayload(payload json.RawMessage) []struct {
	Key   string
	Value string
} {
	if payload == nil {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil
	}

	// Extract all fields as strings
	var fields []struct {
		Key   string
		Value string
	}

	for k, v := range data {
		// Convert value to string representation
		var strValue string
		switch val := v.(type) {
		case string:
			strValue = val
		case bool:
			strValue = fmt.Sprintf("%t", val)
		case float64:
			// If it's an integer, format without decimal
			if val == float64(int(val)) {
				strValue = fmt.Sprintf("%d", int(val))
			} else {
				strValue = fmt.Sprintf("%g", val)
			}
		case nil:
			strValue = "<nil>"
		default:
			// For complex types, use JSON representation
			rawJSON, err := json.Marshal(val)
			if err != nil {
				strValue = fmt.Sprintf("%v", val)
			} else {
				strValue = string(rawJSON)
			}
		}

		fields = append(fields, struct {
			Key   string
			Value string
		}{
			Key:   k,
			Value: strValue,
		})
	}

	// Sort alphabetically by key
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Key < fields[j].Key
	})

	return fields
}

func printJSONWithPrefix(data json.RawMessage, prefix string) {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, "", "  "); err != nil {
		fmt.Printf("%s%s\n", prefix, string(data))
		return
	}

	// Split the JSON into lines
	lines := strings.Split(prettyJSON.String(), "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Printf("%s%s\n", prefix, line)
		}
	}
}

// formatDuration converts duration to human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func formatInspectionError(err string) string {
	if err == "" {
		return "─"
	}
	return err
}

func renderTaskTimeline(statusHistory []api.TaskStatusEvent, interimResults []api.InterimTaskResult) string {
	if len(statusHistory) == 0 || len(interimResults) == 0 {
		return ""
	}

	// Extract key information only
	startTime := statusHistory[0].Timestamp
	endTime := statusHistory[len(statusHistory)-1].Timestamp
	duration := endTime.Sub(startTime).Seconds()

	// Extract latest progress percentage if available
	latestProgress := 0
	for _, result := range interimResults {
		var data map[string]interface{}
		if err := json.Unmarshal(result.Data, &data); err == nil {
			if p, ok := data["progress"].(float64); ok {
				latestProgress = int(p)
			} else if p, ok := data["percentComplete"].(float64); ok {
				latestProgress = int(p)
			}
		}
	}

	// Build simple progress bar (inspired by kubectl)
	var sb strings.Builder
	sb.WriteString("│ Progress: [")

	// Progress bar (20 chars max)
	progressChars := 20
	filledChars := int(float64(latestProgress) / 100 * float64(progressChars))
	sb.WriteString(strings.Repeat("=", filledChars))

	if filledChars < progressChars {
		sb.WriteString(">") // Progress indicator
		sb.WriteString(strings.Repeat(" ", progressChars-filledChars-1))
	}

	sb.WriteString(fmt.Sprintf("] %d%%", latestProgress))

	// Add timing information
	sb.WriteString(fmt.Sprintf("  (%.1fs elapsed)", duration))
	sb.WriteString("\n")

	// Add update count
	sb.WriteString(fmt.Sprintf("│ Updates: %d", len(interimResults)))
	sb.WriteString("\n")

	return sb.String()
}
