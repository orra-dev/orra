package cmd

import (
	"fmt"

	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

func newPsCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List orchestrations for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get project config, using --project flag if provided
			proj, projectID, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			fmt.Printf("Listing orchestrations for project: %s\n", projectID)
			fmt.Printf("Using server: %s\n", proj.ServerAddr)
			// TODO: Implement actual API call
			return nil
		},
	}
}
