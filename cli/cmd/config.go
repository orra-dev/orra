/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package cmd

import (
	"fmt"

	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage config",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newResetConfigCmd(opts))

	return cmd
}

func newResetConfigCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset existing config",
		Long:  "Reset config after control plane restart",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.ResetConfig(opts.ConfigPath); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Config successfully reset, path: %s\n", opts.ConfigPath)
			return nil
		},
	}
}
