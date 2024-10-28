package cmd

import (
	"fmt"

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
		Use:   "create [project-id] --webhook [webhook-url]",
		Short: "Create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := args[0]
			webhook, _ := cmd.Flags().GetString("webhook")
			fmt.Printf("Creating project %s with webhook: %s\n", projectID, webhook)

			// TODO: Call API to create project and get API key
			apiKey := "dummy-api-key"

			if err := config.SaveProject(opts.ConfigPath, projectID, apiKey, serverAddr); err != nil {
				return fmt.Errorf("failed to save project: %w", err)
			}

			fmt.Printf("Project %s created successfully\n", projectID)
			fmt.Printf("API Key: %s\n", apiKey)
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
			for id, proj := range opts.Config.Projects {
				current := " "
				if id == opts.Config.CurrentProject {
					current = "*"
				}
				fmt.Printf("%s %s\n  Server: %s\n", current, id, proj.ServerAddr)
			}
			return nil
		},
	}
}

func newProjectUseCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "use [project-id]",
		Short: "Set the current project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := args[0]

			if _, exists := opts.Config.Projects[projectID]; !exists {
				return fmt.Errorf("project %s not found", projectID)
			}

			opts.Config.CurrentProject = projectID
			if err := config.SaveConfig(opts.ConfigPath, opts.Config); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Now using project: %s\n", projectID)
			return nil
		},
	}
}
