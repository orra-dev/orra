package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
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
)

type column struct {
	header string
	value  func(o api.OrchestrationView) string
}

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
				return fmt.Errorf("failed to list orchestrated actions: %w", err)
			}

			fmt.Printf("Project: %s\nServer: %s\n\n", projectName, proj.ServerAddr)

			if orchestrations.Empty() {
				fmt.Println("No actions have been orchestrated yet")
				return nil
			}

			// Define base columns (used in both views)
			baseColumns := []column{
				{"ORCHESTRATION ID", func(o api.OrchestrationView) string { return o.ID }},
				{"ACTION", func(o api.OrchestrationView) string { return o.Action }},
				{"STATUS", func(o api.OrchestrationView) string { return formatStatus(o.Status.String()) }},
				{"CREATED", func(o api.OrchestrationView) string { return getRelativeTime(o.Timestamp) }},
			}

			// Add ERROR column for wide view
			var columns []column
			if wide {
				columns = append(baseColumns, column{
					"ERROR",
					func(o api.OrchestrationView) string { return formatError(o.Error) },
				})
			} else {
				columns = baseColumns
			}

			// Setup tabwriter
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			defer func(w *tabwriter.Writer) {
				_ = w.Flush()
			}(w)

			// Print headers
			headers := make([]string, len(columns))
			for i, col := range columns {
				headers[i] = col.header
			}
			_, _ = fmt.Fprintln(w, strings.Join(headers, "\t"))

			// Combine and print all orchestrations
			var allOrchestrations []api.OrchestrationView
			allOrchestrations = append(allOrchestrations, orchestrations.Pending...)
			allOrchestrations = append(allOrchestrations, orchestrations.Processing...)
			allOrchestrations = append(allOrchestrations, orchestrations.Completed...)
			allOrchestrations = append(allOrchestrations, orchestrations.Failed...)
			allOrchestrations = append(allOrchestrations, orchestrations.NotActionable...)

			for _, o := range allOrchestrations {
				values := make([]string, len(columns))
				for i, col := range columns {
					values[i] = col.value(o)
				}
				_, _ = fmt.Fprintln(w, strings.Join(values, "\t"))
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&wide, "wide", "w", false, "Show more details including errors")
	return cmd
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
	default:
		return "  " + status // Double space to align with other symbols
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
