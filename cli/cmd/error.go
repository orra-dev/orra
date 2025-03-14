/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var cobraUsageErrorPatterns = []string{
	"unknown command",
	"unknown flag",
	"unknown shorthand flag",
	"invalid argument",
	"requires at least",
	"requires at most",
	"accepts",
	"flag needs",
	"required flag",
}

func handleCommandError(rootCmd *cobra.Command, err error) {
	failedCmd, _, findErr := rootCmd.Find(os.Args[1:])

	if cannotFindFailedCmd := findErr != nil; cannotFindFailedCmd {
		failedCmd = rootCmd
	}

	// Save the terminal state
	rawModeOff := exec.Command("/bin/stty", "-raw", "echo")
	rawModeOff.Stdin = os.Stdin

	// Check if terminal supports ANSI
	var errorPrefix string
	if runtime.GOOS == "windows" {
		// Simple prefix for Windows without ANSI support
		errorPrefix = "x "
	} else {
		// ANSI color for Unix-like systems
		errorPrefix = "\033[31m⨯\033[0m "
	}

	if isUsageError(err) {
		_, _ = fmt.Fprintf(os.Stderr, "%s%s\n\n", errorPrefix, err)
		_ = failedCmd.Usage()
		return
	}

	_, _ = fmt.Fprintf(os.Stderr, "%sError: %s\n", errorPrefix, err)

	return
}

func isUsageError(err error) bool {
	errMsg := err.Error()

	for _, msg := range cobraUsageErrorPatterns {
		if strings.Contains(errMsg, msg) {
			return true
		}
	}

	return false
}
