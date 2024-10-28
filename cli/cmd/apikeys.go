package cmd

import (
	"github.com/spf13/cobra"
)

func newAPIKeysCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api-keys",
		Short: "Add and manage API keys for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newAPIKeyCreateCmd(opts))
	cmd.AddCommand(newAPIKeyListCmd(opts))

	return cmd
}

func newAPIKeyCreateCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "create [project-id]",
		Short: "Create a new API key for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement API key creation logic
			return nil
		},
	}
}

func newAPIKeyListCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "list [project-id]",
		Short: "List all API keys for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement API key listing logic
			return nil
		},
	}
}
