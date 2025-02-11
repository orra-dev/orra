/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

func newProjectsCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Add and manage projects",
		Long:  `Add and manage Orra projects.`,
	}

	cmd.AddCommand(newProjectCreateCmd(opts))
	cmd.AddCommand(newProjectListCmd(opts))
	cmd.AddCommand(newProjectUseCmd(opts))

	return cmd
}

func newProjectCreateCmd(opts *CliOpts) *cobra.Command {
	var serverAddr string

	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Add a new project",
		Long:  `Add a new project so the control plane can orchestrate your app.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]
			if len(strings.TrimSpace(projectName)) == 0 {
				return fmt.Errorf("provide a project name, e.g orra projects new sever-api-key")
			}

			if serverAddr == "" {
				serverAddr = DefaultControlPlaneServerAddr
			}

			if _, exists := opts.Config.Projects[projectName]; exists {
				return fmt.Errorf("project name %s already exists", projectName)
			}

			// Create project in control plane (includes initial API key)
			client := opts.ApiClient.SetBaseUrl(serverAddr)
			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			project, err := client.CreateProject(ctx)
			if err != nil {
				return fmt.Errorf("failed to create project - %w", err)
			}

			if err := config.SaveNewProject(opts.ConfigPath, projectName, project.ID, project.CliAPIKey, serverAddr); err != nil {
				return fmt.Errorf("failed to save new project config: %w", err)
			}

			fmt.Printf("Project %s created successfully\n\n", projectName)
			fmt.Println("To orchestrate your app:")
			fmt.Println("  orra webhooks add [valid webhook url]")
			fmt.Println("  orra api-keys gen [api key name]")
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", "", "Server address (default: http://localhost:8005)")
	return cmd
}

func newProjectListCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(opts.Config.Projects) == 0 {
				fmt.Println("No projects added yet")
				return nil
			}

			fmt.Printf("Current project: %s\n\n", opts.Config.CurrentProject)
			for name, proj := range opts.Config.Projects {
				current := " "
				if name == opts.Config.CurrentProject {
					current = CurrentMarker
				}
				fmt.Printf("%s %s\n  ID: %s\n  Server: %s\n", current, name, proj.ID, proj.ServerAddr)
			}
			return nil
		},
	}
}

func newProjectUseCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "use [name]",
		Short: "Set the current project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]

			if _, exists := opts.Config.Projects[projectName]; !exists {
				return fmt.Errorf("project %s not found", projectName)
			}

			opts.Config.CurrentProject = projectName
			if err := config.SaveConfig(opts.ConfigPath, opts.Config); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Now using project: %s\n", projectName)
			return nil
		},
	}
}
