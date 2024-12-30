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
	"strings"
	"time"

	"github.com/ezodude/orra/cli/internal/api"
	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

func newInspectCmd(opts *CliOpts) *cobra.Command {
	var detailed bool

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
				if _, ok := err.(api.NotFoundError); ok {
					return err
				}
				return fmt.Errorf("failed to inspect orchestration: %w", err)
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

					// Print compensation history if present
					if len(task.CompensationHistory) > 0 {
						fmt.Printf("│ Compensating %s\n", strings.Repeat("─", 39))

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
	return cmd
}

func getStatusSuffix(status api.Status) string {
	switch strings.ToLower(status.String()) {
	case "failed":
		return "[FAILED]"
	case "not_actionable":
		return "[NOT ACTIONABLE]"
	default:
		return ""
	}
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
