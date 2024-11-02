package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

func newAPIKeysCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api-keys",
		Short: "Generate and manage API keys for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newAPIKeyGenerateCmd(opts))
	cmd.AddCommand(newAPIKeyListCmd(opts))

	return cmd
}

func newAPIKeyGenerateCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "gen [name]",
		Short: "Generate an API key for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKeyName := args[0]
			if len(strings.TrimSpace(apiKeyName)) == 0 {
				return fmt.Errorf("provide an API key name, e.g orra api-keys gen sever-api-key")
			}

			projectName, err := getProjectName(opts)
			if err != nil {
				return err
			}

			proj, exists := opts.Config.Projects[projectName]
			if !exists {
				return fmt.Errorf("project %s not found", projectName)
			}

			if _, exists := opts.Config.Projects[projectName].APIKeys[apiKeyName]; exists {
				return fmt.Errorf("API key name %s already exists", apiKeyName)
			}

			client := opts.ApiClient.SetBaseUrl(proj.ServerAddr).SetApiKey(proj.CliAuth)
			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			apiKey, err := client.GenerateAdditionalApiKey(ctx)
			if err != nil {
				return err
			}

			if proj.APIKeys == nil {
				proj.APIKeys = make(map[string]string)
			}
			proj.APIKeys[apiKeyName] = apiKey.APIKey
			opts.Config.Projects[projectName] = proj

			if err := config.SaveConfig(opts.ConfigPath, opts.Config); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Additional API key generated for project %s:\n", projectName)
			fmt.Printf("%s %s\n  KEY: %s\n", ListMarker, apiKeyName, apiKey.APIKey)
			return nil
		},
	}
}

func newAPIKeyListCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all API keys for a project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName, err := getProjectName(opts)
			if err != nil {
				return err
			}

			proj, exists := opts.Config.Projects[projectName]
			if !exists {
				return fmt.Errorf("project %s not found", projectName)
			}

			if proj.APIKeys == nil || len(proj.APIKeys) == 0 {
				fmt.Printf("No API keys generated yet for project %s\n", projectName)
				return nil
			}

			fmt.Printf("API keys for project %s:\n", projectName)
			for name, key := range proj.APIKeys {
				fmt.Printf("%s %s\n  KEY: %s\n", ListMarker, name, key)
			}
			return nil
		},
	}
}
