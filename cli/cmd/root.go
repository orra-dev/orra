package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type CliOpts struct {
	ConfigPath string
}

func NewOrraCommand(opts *CliOpts) *cobra.Command {
	if opts == nil {
		opts = &CliOpts{}
	}

	cmd := &cobra.Command{
		Use:   "orra",
		Short: "Orra CLI for managing orchestration workflows",
		Long: `orra manages Orra and orchestration workflows.
Command line interface for interacting with Orra Control Plane.`,
		Run: runHelp,
	}

	// Add global flags
	cmd.PersistentFlags().StringVar(&opts.ConfigPath, "config", "", "config file (default is $HOME/.orra/config.json)")

	// Add commands
	cmd.AddCommand(newProjectsCmd(opts))
	cmd.AddCommand(newAPIKeysCmd(opts))
	cmd.AddCommand(newPsCmd(opts))
	cmd.AddCommand(newInspectCmd(opts))
	cmd.AddCommand(newLogsCmd(opts))
	cmd.AddCommand(newVersionCmd(opts))

	return cmd
}

func runHelp(cmd *cobra.Command, args []string) {
	cmd.Help()
}

func Execute() {
	opts := &CliOpts{}
	rootCmd := NewOrraCommand(opts)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
