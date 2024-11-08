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

func newWebhooksCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhooks",
		Short: "Add and manage webhooks for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newWebhookAddCmd(opts))
	cmd.AddCommand(newWebhookListCmd(opts))

	return cmd
}

func newWebhookAddCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "add [webhook url]",
		Short: "Add a webhook to the project",
		Long:  "Add a webhook to the project so you can receive orchestration results.",
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

			if contains(proj.Webhooks, webhookUrl) {
				return fmt.Errorf("webhook already exists for project %s", projectName)
			}

			client := opts.ApiClient.SetBaseUrl(proj.ServerAddr).SetApiKey(proj.CliAuth)
			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			webhook, err := client.AddWebhook(ctx, webhookUrl)
			if err != nil {
				return fmt.Errorf("failed to create API key: %w", err)
			}

			proj.Webhooks = append(proj.Webhooks, webhook.Url)
			opts.Config.Projects[projectName] = proj

			if err := config.SaveConfig(opts.ConfigPath, opts.Config); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("New webhook added to project %s:\n", projectName)
			fmt.Printf("Webhook: %s\n", webhook.Url)

			return nil
		},
	}
}

func newWebhookListCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all webhooks for a project",
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

			if len(proj.Webhooks) == 0 {
				fmt.Printf("No webhooks added yet for project %s\n", projectName)
				return nil
			}

			fmt.Printf("Webhooks for project %s:\n\n", projectName)
			for _, webhook := range proj.Webhooks {
				fmt.Printf("%s %s\n", ListMarker, webhook)
			}
			return nil
		},
	}
}
