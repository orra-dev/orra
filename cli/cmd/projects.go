package cmd

import (
	"github.com/spf13/cobra"
)

func newProjectsCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Add and manage projects",
		Long:  `Add and manage Orra projects.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newProjectCreateCmd(opts))
	cmd.AddCommand(newProjectListCmd(opts))

	return cmd
}

func newProjectCreateCmd(opts *CliOpts) *cobra.Command {
	var webhook string

	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement project creation logic
			return nil
		},
	}

	cmd.Flags().StringVar(&webhook, "webhook", "", "Webhook URL for project notifications")
	cmd.MarkFlagRequired("webhook")

	return cmd
}

func newProjectListCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement project listing logic
			return nil
		},
	}
}
