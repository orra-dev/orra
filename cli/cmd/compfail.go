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
