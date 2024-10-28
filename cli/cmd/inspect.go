package cmd

import (
	"github.com/spf13/cobra"
)

func newInspectCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect [orchestration-id]",
		Short: "Return information of an orchestration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement orchestration inspection logic
			return nil
		},
	}
}
