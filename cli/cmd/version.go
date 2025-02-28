/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

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
			fmt.Println("Client Version: v0.2.2")
			// TODO: Implement server version check
			return nil
		},
	}
}
