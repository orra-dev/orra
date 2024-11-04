package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/ezodude/orra/cli/internal/api"
	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

const (
	DefaultControlPlaneServerAddr = "http://localhost:8005"
	CurrentMarker                 = "*"
	ListMarker                    = "-"
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
		Use:           "orra",
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		Long: `🪡 Seamlessly manage and monitor your Orra-powered applications.`,
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
	cmd.PersistentFlags().StringVarP(&opts.ProjectID, "project", "p", "", "project Name (overrides current project)")

	// Add commands
	cmd.AddCommand(newProjectsCmd(opts))
	cmd.AddCommand(newWebhooksCmd(opts))
	cmd.AddCommand(newAPIKeysCmd(opts))
	cmd.AddCommand(newPsCmd(opts))
	cmd.AddCommand(newInspectCmd(opts))
	//cmd.AddCommand(newLogsCmd(opts))
	cmd.AddCommand(newTestCmd(opts))
	cmd.AddCommand(newVersionCmd(opts))
	cmd.AddCommand(newConfigCmd(opts))

	return cmd
}

func Execute() {
	opts := &CliOpts{
		ApiClient: api.NewClient().SetTimeout(30 * time.Second),
	}

	rootCmd := NewOrraCommand(opts)

	if err := rootCmd.Execute(); err != nil {
		handleCommandError(rootCmd, err)
		os.Exit(1)
	}
}

func getProjectName(opts *CliOpts) (string, error) {
	var out string
	if opts.ProjectID != "" {
		out = opts.ProjectID
	} else {
		out = opts.Config.CurrentProject
	}

	if out == "" {
		return "", fmt.Errorf("no project specified")
	}
	return out, nil
}

func contains(entries []string, v string) bool {
	for _, e := range entries {
		if e == v {
			return true
		}
	}
	return false
}
