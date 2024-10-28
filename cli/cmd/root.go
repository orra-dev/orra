package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/ezodude/orra/cli/internal/api"
	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

type CliOpts struct {
	Config     *config.Config
	ConfigPath string
	ProjectID  string // For project override via flag
	ApiClient  *api.Client
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip config loading for version command
			if cmd.Name() == "version" {
				return nil
			}

			cfg, configPath, err := config.LoadOrInit(opts.ConfigPath)
			if err != nil {
				return fmt.Errorf("failed to initialize config: %w", err)
			}
			opts.Config = cfg
			opts.ConfigPath = configPath
			return nil
		},
	}

	// Global flags
	cmd.PersistentFlags().StringVar(&opts.ConfigPath, "config", "", "config file (default is $HOME/.orra/config.json)")
	cmd.PersistentFlags().StringVarP(&opts.ProjectID, "project", "p", "", "Project ID (overrides current project)")

	// Add commands
	cmd.AddCommand(newProjectsCmd(opts))
	cmd.AddCommand(newPsCmd(opts))
	cmd.AddCommand(newInspectCmd(opts))
	cmd.AddCommand(newLogsCmd(opts))
	cmd.AddCommand(newVersionCmd(opts))

	return cmd
}

func Execute() {
	opts := &CliOpts{
		ApiClient: api.NewClient().Timeout(30 * time.Second),
	}

	rootCmd := NewOrraCommand(opts)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
