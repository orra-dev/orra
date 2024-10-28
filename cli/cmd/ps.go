package cmd

import (
	"github.com/spf13/cobra"
)

func newPsCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List orchestrations for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement orchestration listing logic
			return nil
		},
	}
}
