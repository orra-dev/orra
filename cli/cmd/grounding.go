/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ezodude/orra/cli/internal/api"
	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newGroundingCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grounding",
		Short: "Ground execution planning in a project's domain",
	}

	cmd.AddCommand(
		newGroundingApplyCmd(opts),
		newGroundingListCmd(opts),
		newGroundingRemoveCmd(opts),
	)

	return cmd
}

func newGroundingApplyCmd(opts *CliOpts) *cobra.Command {
	var filename string

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply grounding to a project",
		Long: `Apply a grounding spec to a project. The spec can be re-applied if the version has changed.
Example:
  orra grounding apply -f customer-support.grounding.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get project config
			proj, projectName, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			// Read and parse spec file
			data, err := os.ReadFile(filename)
			if err != nil {
				return fmt.Errorf("failed to read grounding spec file: %w", err)
			}

			var toApply api.GroundingSpec
			if err := yaml.Unmarshal(data, &toApply); err != nil {
				return fmt.Errorf("failed to parse grounding spec file: %w", err)
			}

			// Setup API client
			client := opts.ApiClient.
				SetBaseUrl(proj.ServerAddr).
				SetApiKey(proj.CliAuth)

			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			if _, err := client.ApplyGroundingSpec(ctx, toApply); err != nil {
				return fmt.Errorf("failed to apply grounding spec from file [%s]: %w", filename, err)
			}

			fmt.Printf("✓ Applied grounding spec from %s to project %s\n", filename, projectName)
			return nil
		},
	}

	cmd.Flags().StringVarP(&filename, "filename", "f", "", "Filename of the grounding spec to apply")
	_ = cmd.MarkFlagRequired("filename")

	return cmd
}

func newGroundingListCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List all groundings in a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, projectName, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			client := opts.ApiClient.
				SetBaseUrl(proj.ServerAddr).
				SetApiKey(proj.CliAuth)

			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			specs, err := client.ListGroundingSpecs(ctx)
			if err != nil {
				return fmt.Errorf("failed to list grounding specs: %w", err)
			}

			// Project Info Section
			fmt.Printf("Project: %s\nServer:  %s\n", projectName, proj.ServerAddr)

			if len(specs) == 0 {
				fmt.Println("\nNo grounding specs found")
				return nil
			}

			// Define columns for the table
			columns := []groundingColumn{
				{"NAME", func(s api.GroundingSpec) string { return s.Name }, 30},
				{"DOMAIN", func(s api.GroundingSpec) string { return s.Domain }, 30},
				{"VERSION", func(s api.GroundingSpec) string { return s.Version }, 15},
				{"USE CASES", func(s api.GroundingSpec) string { return fmt.Sprintf("%d", len(s.UseCases)) }, 10},
			}

			// Print table with styling
			fmt.Printf("\n┌─ Grounding\n")

			// Header
			headerFmt := buildFormatStringGC(columns)
			fmt.Printf("│ "+headerFmt+"\n", toInterfaceSlice(getHeadersGC(columns))...)
			fmt.Printf("│ %s\n", strings.Repeat("─", calculateLineWidthGC(columns)))

			// Rows
			for _, spec := range specs {
				values := make([]interface{}, len(columns))
				for i, col := range columns {
					values[i] = col.value(spec)
				}
				fmt.Printf("│ "+headerFmt+"\n", values...)
			}
			fmt.Printf("└─────\n")

			return nil
		},
	}

	return cmd
}

func newGroundingRemoveCmd(opts *CliOpts) *cobra.Command {
	var removeAll bool

	cmd := &cobra.Command{
		Use:     "rm [name]",
		Aliases: []string{"remove"},
		Short:   "Remove grounding from a project",
		Long: `Remove grounding from a project. Specify a name to remove a single spec,
or use --all to remove all grounding.

Example:
  orra grounding rm customer-support-examples
  orra grounding rm --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, projectName, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			client := opts.ApiClient.
				SetBaseUrl(proj.ServerAddr).
				SetApiKey(proj.CliAuth)

			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			if removeAll {
				if err := client.RemoveAllGroundingSpecs(ctx); err != nil {
					return fmt.Errorf("failed to remove all grounding specs: %w", err)
				}
				fmt.Printf("✓ Removed all grounding specs from project %s\n", projectName)
				return nil
			}

			if len(args) != 1 {
				return fmt.Errorf("requires a grounding spec name argument")
			}

			if err := client.RemoveGroundingSpec(ctx, args[0]); err != nil {
				var notFoundError api.NotFoundError
				if errors.As(err, &notFoundError) {
					return err
				}
				return fmt.Errorf("failed to remove grounding spec '%s': %w", args[0], err)
			}

			fmt.Printf("✓ Removed grounding spec '%s' from project %s\n", args[0], projectName)
			return nil
		},
	}

	cmd.Flags().BoolVar(&removeAll, "all", false, "Remove all grounding specs")

	return cmd
}

type groundingColumn struct {
	header string
	value  func(o api.GroundingSpec) string
	width  int
}

func calculateLineWidthGC(columns []groundingColumn) int {
	width := len(columns) + 1 // Account for spacing between columns
	for _, col := range columns {
		width += col.width
	}
	return width
}

func buildFormatStringGC(columns []groundingColumn) string {
	formats := make([]string, len(columns))
	for i, col := range columns {
		formats[i] = fmt.Sprintf("%%-%ds", col.width)
	}
	return strings.Join(formats, " ")
}

func getHeadersGC(columns []groundingColumn) []string {
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = col.header
	}
	return headers
}
