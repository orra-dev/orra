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
	return &cobra.Command{
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

			// Project Info Section
			fmt.Printf("Project: %s\nServer:  %s\n", projectName, proj.ServerAddr)

			if len(compensations) == 0 {
				fmt.Println("\nNo failed compensations found")
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
			fmt.Printf("\n┌─ Failed Compensations\n")

			// Header
			headerFmt := buildCompFailFormatString(columns)
			fmt.Printf("│ "+headerFmt+"\n", toInterfaceSlice(getCompFailHeaders(columns))...)
			fmt.Printf("│ %s\n", strings.Repeat("─", calculateCompFailLineWidth(columns)))

			// Rows
			for _, comp := range compensations {
				values := make([]interface{}, len(columns))
				for i, col := range columns {
					values[i] = col.value(comp)
				}
				fmt.Printf("│ "+headerFmt+"\n", values...)
			}
			fmt.Printf("└─────\n")

			return nil
		},
	}
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
