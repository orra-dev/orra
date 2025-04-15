/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ezodude/orra/cli/internal/api"
	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

const (
	symbolPending       = "○ " // Empty circle for pending
	symbolProcessing    = "◎ " // Dotted circle for processing
	symbolCompleted     = "● " // Filled circle for completed
	symbolFailed        = "✕ " // Cross for failed
	symbolNotActionable = "⊘ " // Prohibited circle for not actionable
	symbolPaused        = "⏸ " // Pause icon for paused
	symbolAborted       = "⊟ " // Crossed box for aborted
)

func newPsCmd(opts *CliOpts) *cobra.Command {
	var wide bool

	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List orchestrated actions for a project",
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

			orchestrations, err := client.ListOrchestrations(ctx)
			if err != nil {
				return fmt.Errorf("failed to list orchestrated actions - %w", err)
			}

			// Project Info Section
			fmt.Printf("Project: %s\nServer:  %s\n", projectName, proj.ServerAddr)

			if len(opts.Config.Projects) == 0 {
				fmt.Println("\nNo actions have been orchestrated yet")
				return nil
			}

			// Define columns based on view mode
			columns := []psColumn{
				{"ID", func(o api.OrchestrationView) string { return o.ID }, 25},
				{"ACTION", func(o api.OrchestrationView) string { return truncateString(o.Action, 34) }, 36},
				{"STATUS", func(o api.OrchestrationView) string { return formatStatus(o.Status.String()) }, 18},
				{"COMPENSATIONS", func(o api.OrchestrationView) string { return formatCompensationSummary(o.Compensation.String()) }, 20},
				{"CREATED", func(o api.OrchestrationView) string { return getRelativeTime(o.Timestamp) }, 10},
			}

			if wide {
				columns = append(columns, psColumn{
					"ERROR",
					func(o api.OrchestrationView) string { return truncateString(formatListError(o.Error), 35) },
					37,
				})
			}

			// Prepare all orchestrations in order: Processing, Pending, Completed, Failed, NotActionable
			var allOrchestrations []api.OrchestrationView
			allOrchestrations = append(allOrchestrations, orchestrations.Processing...)
			allOrchestrations = append(allOrchestrations, orchestrations.Pending...)
			allOrchestrations = append(allOrchestrations, orchestrations.Completed...)
			allOrchestrations = append(allOrchestrations, orchestrations.Failed...)
			allOrchestrations = append(allOrchestrations, orchestrations.NotActionable...)

			if len(allOrchestrations) == 0 {
				fmt.Println("\nNo orchestrations found")
				return nil
			}

			// Print table with new styling
			fmt.Printf("\n┌─ Orchestrations\n")

			// Header
			headerFmt := buildFormatString(columns)
			fmt.Printf("│ "+headerFmt+"\n", toInterfaceSlice(getHeaders(columns))...)
			fmt.Printf("│ %s\n", strings.Repeat("─", calculateLineWidth(columns)))

			// Rows
			for _, o := range allOrchestrations {
				values := make([]interface{}, len(columns))
				for i, col := range columns {
					values[i] = col.value(o)
				}
				fmt.Printf("│ "+headerFmt+"\n", values...)
			}
			fmt.Printf("└─────\n")

			return nil
		},
	}

	cmd.Flags().BoolVarP(&wide, "wide", "w", false, "Show more details including errors")
	return cmd
}

type psColumn struct {
	header string
	value  func(o api.OrchestrationView) string
	width  int
}

func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}

func calculateLineWidth(columns []psColumn) int {
	width := len(columns) + 1 // Account for spacing between columns
	for _, col := range columns {
		width += col.width
	}
	return width
}

func buildFormatString(columns []psColumn) string {
	formats := make([]string, len(columns))
	for i, col := range columns {
		formats[i] = fmt.Sprintf("%%-%ds", col.width)
	}
	return strings.Join(formats, " ")
}

func getHeaders(columns []psColumn) []string {
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = col.header
	}
	return headers
}

func toInterfaceSlice(s []string) []interface{} {
	is := make([]interface{}, len(s))
	for i, v := range s {
		is[i] = v
	}
	return is
}

func formatListError(err json.RawMessage) string {
	if len(err) == 0 {
		return "─"
	}
	return string(err)
}

func formatStatus(status string) string {
	switch strings.ToLower(status) {
	case "pending":
		return symbolPending + status
	case "processing":
		return symbolProcessing + status
	case "paused":
		return symbolPaused + status
	case "completed":
		return symbolCompleted + status
	case "failed":
		return symbolFailed + status
	case "not actionable":
		return symbolNotActionable + status
	case "aborted":
		return symbolAborted + status
	default:
		return "  " + status // Double space to align with other symbols
	}
}

func formatCompensationSummary(summary string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(summary), "active"):
		return symbolProcessing + summary
	case strings.HasPrefix(strings.ToLower(summary), "completed"):
		return symbolCompleted + summary
	case strings.HasPrefix(strings.ToLower(summary), "failed"):
		return symbolFailed + summary
	default:
		return "─"
	}
}

func formatCompensation(status string) string {
	s := strings.ToLower(status)
	switch {
	case strings.Contains(s, "pending"):
		return symbolPending + status
	case strings.Contains(s, "processing"):
		return symbolProcessing + status
	case strings.Contains(s, "completed"):
		return symbolCompleted + status
	case strings.Contains(s, "expired"), strings.Contains(s, "failed"):
		return symbolFailed + status
	default:
		return "─"
	}
}

func getRelativeTime(t time.Time) string {
	duration := time.Since(t)
	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
}
