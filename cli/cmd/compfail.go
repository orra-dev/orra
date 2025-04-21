/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/ezodude/orra/cli/internal/api"
	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

func newCompFailCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comp-fail",
		Short: "Manage compensation failures",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newCompFailWebhooksCmd(opts))
	cmd.AddCommand(newCompFailListCmd(opts))
	cmd.AddCommand(newCompFailInspectCmd(opts))
	cmd.AddCommand(newCompFailResolveCmd(opts))
	cmd.AddCommand(newCompFailIgnoreCmd(opts))

	return cmd
}

func newCompFailWebhooksCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhooks",
		Short: "Manage compensation failure webhooks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newCompFailWebhookAddCmd(opts))
	cmd.AddCommand(newCompFailWebhookListCmd(opts))

	return cmd
}

func newCompFailWebhookAddCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "add [webhook url]",
		Short: "Add a compensation failure webhook to the project",
		Long:  "Add a webhook to receive notifications when compensation operations fail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName, err := getProjectName(opts)
			if err != nil {
				return err
			}

			proj, exists := opts.Config.Projects[projectName]
			if !exists {
				return fmt.Errorf("project %s not found", projectName)
			}

			webhookUrl := args[0]
			if _, err := url.ParseRequestURI(webhookUrl); err != nil {
				return fmt.Errorf("invalid webhook, it should be a valid url")
			}

			if contains(proj.CompensationFailureWebhooks, webhookUrl) {
				return fmt.Errorf("compensation failure webhook already exists for project %s", projectName)
			}

			client := opts.ApiClient.SetBaseUrl(proj.ServerAddr).SetApiKey(proj.CliAuth)
			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			webhook, err := client.AddCompensationFailureWebhook(ctx, webhookUrl)
			if err != nil {
				return fmt.Errorf("failed to add compensation failure webhook - %w", err)
			}

			// Update the project's webhooks list
			if proj.CompensationFailureWebhooks == nil {
				proj.CompensationFailureWebhooks = []string{}
			}
			proj.CompensationFailureWebhooks = append(proj.CompensationFailureWebhooks, webhook.Url)
			opts.Config.Projects[projectName] = proj

			if err := config.SaveConfig(opts.ConfigPath, opts.Config); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("New compensation failure webhook added to project %s:\n", projectName)
			fmt.Printf("Webhook: %s\n", webhook.Url)

			return nil
		},
	}
}

func newCompFailWebhookListCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all compensation failure webhooks for a project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName, err := getProjectName(opts)
			if err != nil {
				return err
			}

			proj, exists := opts.Config.Projects[projectName]
			if !exists {
				return fmt.Errorf("project %s not found", projectName)
			}

			if len(proj.CompensationFailureWebhooks) == 0 {
				fmt.Printf("No compensation failure webhooks added yet for project %s\n", projectName)
				return nil
			}

			fmt.Printf("Compensation failure webhooks for project %s:\n\n", projectName)
			for _, webhook := range proj.CompensationFailureWebhooks {
				fmt.Printf("%s %s\n", ListMarker, webhook)
			}
			return nil
		},
	}
}

// Define symbols for the status and resolution state columns
const (
	symbolResolutionPending  = "○ " // Empty circle for pending resolution
	symbolResolutionResolved = "● " // Filled circle for resolved
	symbolResolutionIgnored  = "⊖ " // Circled minus for ignored
)

func newCompFailListCmd(opts *CliOpts) *cobra.Command {
	var (
		orchestrationFlag string
		serviceFlag       string
		resolutionFlag    string
		limitFlag         int
		showAllFlag       bool
	)

	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List failed compensations for a project",
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

			compensations, err := client.ListFailedCompensations(ctx)
			if err != nil {
				return fmt.Errorf("failed to list compensation failures - %w", err)
			}

			// Get all compensations before filtering
			totalCompensations := len(compensations)

			// Filter by orchestration and service first
			prefiltered := prefilterCompensations(compensations, orchestrationFlag, serviceFlag)

			// Then handle resolution state filtering
			var filtered []api.FailedCompensation

			// Case 1: If --resolution flag is used, it takes precedence
			if resolutionFlag != "" {
				filtered = filterByResolution(prefiltered, resolutionFlag)
			} else if showAllFlag {
				// Case 2: If --all flag is used, show everything (after pre-filtering)
				filtered = prefiltered
			} else {
				// Case 3: Default behavior - show only pending compensations
				filtered = filterByResolution(prefiltered, "pending")
			}

			// Apply limit (if not zero)
			if limitFlag > 0 && limitFlag < len(filtered) {
				filtered = filtered[:limitFlag]
			}

			// Project Info Section
			fmt.Printf("Project: %s\nServer:  %s\n", projectName, proj.ServerAddr)

			// Show filter information if any filters are applied
			hasFilters := orchestrationFlag != "" || serviceFlag != "" || resolutionFlag != "" || showAllFlag
			if hasFilters {
				fmt.Println("\nApplied filters:")
				if orchestrationFlag != "" {
					fmt.Printf("  Orchestration: %s\n", orchestrationFlag)
				}
				if serviceFlag != "" {
					fmt.Printf("  Service: %s\n", serviceFlag)
				}
				if resolutionFlag != "" {
					fmt.Printf("  Resolution: %s\n", resolutionFlag)
				} else if showAllFlag {
					fmt.Printf("  Resolution: all states\n")
				} else {
					fmt.Printf("  Resolution: pending only (default)\n")
				}
			}

			// Count how many of each resolution state we have in the prefiltered results
			hiddenResolved := 0
			hiddenIgnored := 0

			// Only count hidden items if we're not showing all and not explicitly filtering by resolution
			if !showAllFlag && resolutionFlag == "" {
				for _, comp := range prefiltered {
					if strings.ToLower(comp.ResolutionState) == "resolved" {
						hiddenResolved++
					} else if strings.ToLower(comp.ResolutionState) == "ignored" {
						hiddenIgnored++
					}
				}
			}

			if len(filtered) == 0 {
				if totalCompensations > 0 {
					if len(prefiltered) > 0 && hiddenResolved+hiddenIgnored > 0 {
						fmt.Println("\nNo pending compensations match the applied filters\n")
						fmt.Printf("There are %d resolved and %d ignored compensations that match your other filters.\n",
							hiddenResolved, hiddenIgnored)
						fmt.Println("Use --all flag to include them, or --resolution=resolved/ignored to see specific states.")
					} else {
						fmt.Println("\nNo failed compensations match the applied filters")
					}
				} else {
					fmt.Println("\nNo failed compensations found")
				}
				return nil
			}

			// Define columns for the table
			columns := []compFailColumn{
				{"ID", func(c api.FailedCompensation) string { return c.ID }, 25},
				{"ORCHESTRATION", func(c api.FailedCompensation) string { return c.OrchestrationID }, 25},
				{"SERVICE", func(c api.FailedCompensation) string { return c.ServiceName }, 30},
				{"STATUS", func(c api.FailedCompensation) string { return formatCompensationStatus(c.Status) }, 13},
				{"RESOLUTION", func(c api.FailedCompensation) string { return formatResolutionState(c.ResolutionState) }, 20},
				{"CREATED", func(c api.FailedCompensation) string { return getRelativeTime(c.Timestamp) }, 10},
			}

			// Print table with styling
			fmt.Printf("\n┌─ Failed Compensations (%d", len(filtered))
			if hiddenResolved > 0 || hiddenIgnored > 0 {
				fmt.Printf(" pending of %d total", hiddenResolved+hiddenIgnored+len(filtered))
			}
			fmt.Printf(")\n")

			// Header
			headerFmt := buildCompFailFormatString(columns)
			fmt.Printf("│ "+headerFmt+"\n", toInterfaceSlice(getCompFailHeaders(columns))...)
			fmt.Printf("│ %s\n", strings.Repeat("─", calculateCompFailLineWidth(columns)))

			// Rows
			for _, comp := range filtered {
				values := make([]interface{}, len(columns))
				for i, col := range columns {
					values[i] = col.value(comp)
				}
				fmt.Printf("│ "+headerFmt+"\n", values...)
			}
			fmt.Printf("└─────\n")

			// Show message about hidden items if applicable
			if (hiddenResolved > 0 || hiddenIgnored > 0) && resolutionFlag == "" && !showAllFlag {
				fmt.Printf("\nNote: %d resolved and %d ignored compensations are hidden.\n",
					hiddenResolved, hiddenIgnored)
				fmt.Println("Use --all to show all states, or --resolution=resolved/ignored for specific states.")
			}

			// Show message if results were limited
			if limitFlag > 0 && len(filtered) == limitFlag && len(prefiltered) > limitFlag {
				fmt.Printf("Results limited to %d items. Use --limit to see more.\n", limitFlag)
			}

			return nil
		},
	}

	// Add filter flags
	cmd.Flags().StringVarP(&orchestrationFlag, "orchestration", "o", "", "Filter by orchestration ID")
	cmd.Flags().StringVarP(&serviceFlag, "service", "s", "", "Filter by service name")
	cmd.Flags().StringVarP(&resolutionFlag, "resolution", "r", "", "Filter by resolution state (pending, resolved, ignored)")
	cmd.Flags().IntVarP(&limitFlag, "limit", "l", 20, "Limit the number of results (default shows last 20)")
	cmd.Flags().BoolVar(&showAllFlag, "all", false, "Show all compensations, including resolved and ignored ones")

	return cmd
}

// prefilterCompensations applies orchestration and service filters only
func prefilterCompensations(compensations []api.FailedCompensation, orchestrationID, serviceName string) []api.FailedCompensation {
	// If no filters applied, return all compensations
	if orchestrationID == "" && serviceName == "" {
		return compensations
	}

	var filtered []api.FailedCompensation
	for _, comp := range compensations {
		// Check orchestration ID filter
		if orchestrationID != "" && !strings.Contains(comp.OrchestrationID, orchestrationID) {
			continue
		}

		// Check service name filter
		if serviceName != "" && !strings.Contains(strings.ToLower(comp.ServiceName), strings.ToLower(serviceName)) {
			continue
		}

		// All filters passed, add to filtered list
		filtered = append(filtered, comp)
	}

	return filtered
}

// filterByResolution applies only resolution state filtering
func filterByResolution(compensations []api.FailedCompensation, resolutionState string) []api.FailedCompensation {
	if resolutionState == "" {
		return compensations
	}

	var filtered []api.FailedCompensation
	for _, comp := range compensations {
		// Check resolution state filter
		if !strings.EqualFold(comp.ResolutionState, resolutionState) {
			continue
		}

		// Filter passed, add to filtered list
		filtered = append(filtered, comp)
	}

	return filtered
}

type compFailColumn struct {
	header string
	value  func(c api.FailedCompensation) string
	width  int
}

func calculateCompFailLineWidth(columns []compFailColumn) int {
	width := len(columns) + 1 // Account for spacing between columns
	for _, col := range columns {
		width += col.width
	}
	return width
}

func buildCompFailFormatString(columns []compFailColumn) string {
	formats := make([]string, len(columns))
	for i, col := range columns {
		formats[i] = fmt.Sprintf("%%-%ds", col.width)
	}
	return strings.Join(formats, " ")
}

func getCompFailHeaders(columns []compFailColumn) []string {
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = col.header
	}
	return headers
}

func formatCompensationStatus(status string) string {
	switch strings.ToLower(status) {
	case "pending":
		return symbolPending + status
	case "processing":
		return symbolProcessing + status
	case "completed":
		return symbolCompleted + status
	case "failed":
		return symbolFailed + status
	case "partial":
		return symbolProcessing + status // Use processing symbol for partial
	case "expired":
		return symbolFailed + status // Use failed symbol for expired
	default:
		return "  " + status // Double space to align with other symbols
	}
}

func formatResolutionState(state string) string {
	switch strings.ToLower(state) {
	case "pending":
		return symbolResolutionPending + state
	case "resolved":
		return symbolResolutionResolved + state
	case "ignored":
		return symbolResolutionIgnored + state
	default:
		return "  " + state // Double space to align with other symbols
	}
}

// newCompFailInspectCmd returns a new inspect command for compensation failures
func newCompFailInspectCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect [compensation-id]",
		Short: "Show detailed information about a compensation failure",
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

			compensationID := args[0]
			comp, err := client.GetFailedCompensation(ctx, compensationID)
			if err != nil {
				return fmt.Errorf("failed to get compensation failure details - %w", err)
			}

			// Project Info Section
			fmt.Printf("Project: %s\nServer:  %s\n", projectName, proj.ServerAddr)

			// Compensation Overview
			fmt.Printf("\n┌─ Compensation Failure\n")
			fmt.Printf("│ ID:             %s\n", comp.ID)
			fmt.Printf("│ Orchestration:  %s\n", comp.OrchestrationID)
			fmt.Printf("│ Service:        %s (%s)\n", comp.ServiceName, comp.ServiceID)
			fmt.Printf("│ Task:           %s\n", comp.TaskID)
			fmt.Printf("│ Status:         %s\n", formatCompensationStatus(comp.Status))
			fmt.Printf("│ Resolution:     %s\n", formatResolutionState(comp.ResolutionState))
			fmt.Printf("│ Failed:         %s ago (%s)\n",
				getRelativeTime(comp.Timestamp),
				comp.Timestamp.Format("2006-01-02 15:04:05"))
			fmt.Printf("│ Attempts:       %d of %d\n", comp.AttemptsMade, comp.MaxAttempts)
			fmt.Printf("└─────\n")

			// Failure Information
			fmt.Printf("\n┌─ Failure Information\n")
			fmt.Printf("│ Error: %s\n", comp.Failure)

			// Add an explanation for error format if there's a colon in the error
			if strings.Contains(comp.Failure, ":") {
				parts := strings.SplitN(comp.Failure, ":", 2)
				fmt.Printf("│\n│ The error above contains:\n")
				fmt.Printf("│   - System info: \"%s\"\n", strings.TrimSpace(parts[0]))
				fmt.Printf("│   - Service error: \"%s\"\n", strings.TrimSpace(parts[1]))
			}
			fmt.Printf("└─────\n")

			// Compensation Context
			if comp.CompensationData.Context != nil {
				ctx := comp.CompensationData.Context
				fmt.Printf("\n┌─ Compensation Context\n")
				fmt.Printf("│ Triggered by:   [%s] Orchestration\n", ctx.Reason)
				fmt.Printf("│ Timestamp:      %s\n", ctx.Timestamp.Format("2006-01-02 15:04:05"))

				// Show context payload if available
				if ctx.Payload != nil && len(ctx.Payload) > 0 {
					fmt.Printf("│\n│ Payload:\n")
					printJSONWithPrefix(ctx.Payload, "│   ")
				}
				fmt.Printf("└─────\n")
			}

			// Compensation Data
			if comp.CompensationData.Input != nil {
				fmt.Printf("\n┌─ Compensation Log\n")
				fmt.Printf("│\n│ Payload used for compensation attempt:\n")
				printJSONWithPrefix(comp.CompensationData.Input, "│   ")
				fmt.Printf("└─────\n")
			}

			// Related Information
			fmt.Printf("\n┌─ Related Information\n")
			fmt.Printf("│\n│ View orchestration details:\n")
			fmt.Printf("│   orra inspect -d %s\n", comp.OrchestrationID)
			fmt.Printf("└─────\n")

			// Resolution Management
			fmt.Printf("\n┌─ Management Options\n")
			fmt.Printf("│\n│ Commands:\n")
			fmt.Printf("│   Mark as resolved:\n")
			fmt.Printf("│     orra comp-fail resolve %s --reason \"Manually fixed\"\n", comp.ID)
			fmt.Printf("│\n│   Ignore this failure:\n")
			fmt.Printf("│     orra comp-fail ignore %s --reason \"Test transaction\"\n", comp.ID)
			fmt.Printf("└─────\n")

			return nil
		},
	}

	return cmd
}

// newCompFailResolveCmd returns a new command to resolve a failed compensation
func newCompFailResolveCmd(opts *CliOpts) *cobra.Command {
	var reasonFlag string

	cmd := &cobra.Command{
		Use:   "resolve [compensation-id]",
		Short: "Mark a failed compensation as resolved",
		Long:  "Mark a failed compensation as resolved with a reason/explanation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, projectName, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			// Validate reason
			if reasonFlag == "" {
				return fmt.Errorf("reason is required, use --reason flag")
			}

			client := opts.ApiClient.
				SetBaseUrl(proj.ServerAddr).
				SetApiKey(proj.CliAuth)

			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			compensationID := args[0]

			// Resolve the compensation
			updatedComp, err := client.ResolveFailedCompensation(ctx, compensationID, reasonFlag)
			if err != nil {
				return fmt.Errorf("failed to resolve compensation failure - %w", err)
			}
			// Project Info Section
			fmt.Printf("Project: %s\nServer:  %s\n", projectName, proj.ServerAddr)

			// Show success message with details
			fmt.Printf("✓ Successfully resolved compensation failure\n")
			fmt.Printf("ID:             %s\n", updatedComp.ID)
			fmt.Printf("Service:        %s\n", updatedComp.ServiceName)
			fmt.Printf("Resolution:     %s\n", formatResolutionState(updatedComp.ResolutionState))
			fmt.Printf("Reason:         %s\n", updatedComp.Resolution)

			return nil
		},
	}

	// Add reason flag
	cmd.Flags().StringVarP(&reasonFlag, "reason", "r", "", "Reason for resolving the compensation failure (required)")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

// newCompFailIgnoreCmd returns a new command to ignore a failed compensation
func newCompFailIgnoreCmd(opts *CliOpts) *cobra.Command {
	var reasonFlag string

	cmd := &cobra.Command{
		Use:   "ignore [compensation-id]",
		Short: "Mark a failed compensation as ignored",
		Long:  "Mark a failed compensation as ignored so no further action is required",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, projectName, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			// Validate reason
			if reasonFlag == "" {
				return fmt.Errorf("reason is required, use --reason flag")
			}

			client := opts.ApiClient.
				SetBaseUrl(proj.ServerAddr).
				SetApiKey(proj.CliAuth)

			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			compensationID := args[0]

			// Ignore the compensation
			updatedComp, err := client.IgnoreFailedCompensation(ctx, compensationID, reasonFlag)
			if err != nil {
				return fmt.Errorf("failed to ignore compensation failure - %w", err)
			}

			// Project Info Section
			fmt.Printf("Project: %s\nServer:  %s\n", projectName, proj.ServerAddr)

			// Show success message with details
			fmt.Printf("✓ Successfully ignored compensation failure\n")
			fmt.Printf("ID:             %s\n", updatedComp.ID)
			fmt.Printf("Service:        %s\n", updatedComp.ServiceName)
			fmt.Printf("Resolution:     %s\n", formatResolutionState(updatedComp.ResolutionState))
			fmt.Printf("Reason:         %s\n", updatedComp.Resolution)

			return nil
		},
	}

	// Add reason flag
	cmd.Flags().StringVarP(&reasonFlag, "reason", "r", "", "Reason for ignoring the compensation failure (required)")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}
