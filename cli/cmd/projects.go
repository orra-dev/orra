package cmd

import (
	"context"
	"fmt"
	"net/url"

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
		Use:   "create [name] --webhook [webhook-url]",
		Short: "Create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]
			webhook, _ := cmd.Flags().GetString("webhook")

			_, err := url.Parse(webhook)
			if err != nil {
				return fmt.Errorf("project name %s already exists", projectName)
			}

			if serverAddr == "" {
				serverAddr = "http://localhost:8005"
			}

			// Check if project name already exists
			if _, exists := opts.Config.Projects[projectName]; exists {
				return fmt.Errorf("project name %s already exists", projectName)
			}

			// Create project in control plane
			client := opts.ApiClient.BaseUrl(serverAddr)
			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			project, err := client.CreateProject(ctx, webhook)
			if err != nil {
				return fmt.Errorf("failed to create project: %w", err)
			}

			// Save project with user-friendly name mapping to control plane UUID
			if err := config.SaveProject(opts.ConfigPath, projectName, project.ID, project.APIKey, serverAddr); err != nil {
				return fmt.Errorf("failed to save project config: %w", err)
			}

			fmt.Printf("Project %s created successfully (ID: %s)\n", projectName, project.ID)
			fmt.Printf("API Key: %s\n", project.APIKey)
			return nil
		},
	}

	cmd.Flags().String("webhook", "", "Webhook URL for project notifications")
	cmd.Flags().StringVar(&serverAddr, "server", "", "Server address (default: http://localhost:8005)")
	cmd.MarkFlagRequired("webhook")

	return cmd
}

func newProjectListCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(opts.Config.Projects) == 0 {
				fmt.Println("No projects configured")
				return nil
			}

			fmt.Printf("Current project: %s\n\n", opts.Config.CurrentProject)
			for name, proj := range opts.Config.Projects {
				current := " "
				if name == opts.Config.CurrentProject {
					current = "*"
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
