package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(_ *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the client and server version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Client Version: v0.1.0-narwhal")
			// TODO: Implement server version check
			return nil
		},
	}
}
