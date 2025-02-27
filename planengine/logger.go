/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gilcrest/diygoapi/logger"
	_ff "github.com/peterbourgon/ff/v3"
	"github.com/rs/zerolog"
)

const (
	// log level environment variable name
	loglevelEnv string = "LOG_LEVEL"
	// minimum accepted log level environment variable name
	logLevelMinEnv string = "LOG_LEVEL_MIN"
	// log error stack environment variable name
	logErrorStackEnv string = "LOG_ERROR_STACK"
)

type flags struct {
	// log-level flag allows for setting logging level, e.g. to run the server
	// with level set to debug, it'd be: ./server -log-level=debug
	// If not set, defaults to error
	loglvl string

	// log-level-min flag sets the minimum accepted logging level
	// - e.g. in production, you may have a policy to never allow logs at
	// trace level. You could set the minimum log level to Debug. Even
	// if the Global log level is set to Trace, only logs at Debug
	// and above would be logged. Default level is trace.
	logLvlMin string

	// logErrorStack flag determines whether or not a full error stack
	// should be logged. If true, error stacks are logged, if false,
	// just the error is logged
	logErrorStack bool
}

// newFlags parses the command line flags using ff and returns
// a flags struct or an error
func newFlags(args []string) (flags, error) {
	// create new FlagSet using the program name being executed (args[0])
	// as the name of the FlagSet
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	var (
		logLvlMin     = fs.String("log-level-min", "trace", fmt.Sprintf("sets minimum log level (trace, debug, info, warn, error, fatal, panic, disabled), (also via %s)", logLevelMinEnv))
		loglvl        = fs.String("log-level", "info", fmt.Sprintf("sets log level (trace, debug, info, warn, error, fatal, panic, disabled), (also via %s)", loglevelEnv))
		logErrorStack = fs.Bool("log-error-stack", true, fmt.Sprintf("if true, log full error stacktrace, else just log error, (also via %s)", logErrorStackEnv))
	)

	// Parse the command line flags from above
	err := _ff.Parse(fs, args[1:], _ff.WithEnvVars())
	if err != nil {
		return flags{}, err
	}

	return flags{
		loglvl:        *loglvl,
		logLvlMin:     *logLvlMin,
		logErrorStack: *logErrorStack,
	}, nil
}

func NewLogger(args []string) (zerolog.Logger, error) {
	var flgs flags
	flgs, err := newFlags(args)
	if err != nil {
		return zerolog.Logger{}, err
	}

	// determine minimum logging level based on flag input
	var minlvl zerolog.Level
	minlvl, err = zerolog.ParseLevel(flgs.logLvlMin)
	if err != nil {
		return zerolog.Logger{}, err
	}

	// determine logging level based on flag input
	var lvl zerolog.Level
	lvl, err = zerolog.ParseLevel(flgs.loglvl)
	if err != nil {
		return zerolog.Logger{}, err
	}

	// setup logger with appropriate defaults
	lgr := logger.NewWithGCPHook(os.Stdout, minlvl, true)

	// logs will be written at the level set in NewLogger (which is
	// also the minimum level). If the logs are to be written at a
	// different level than the minimum, use SetGlobalLevel to set
	// the global logging level to that. Minimum rules will still
	// apply.
	if minlvl != lvl {
		zerolog.SetGlobalLevel(lvl)
	}

	// set global logging time field format to Unix timestamp
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	lgr.Info().Msgf("minimum accepted logging level set to %s", minlvl)
	lgr.Info().Msgf("logging level set to %s", lvl)

	// set global to log errors with stack (or not) based on flag
	logger.LogErrorStackViaPkgErrors(flgs.logErrorStack)
	lgr.Info().Msgf("log error stack global set to %t", flgs.logErrorStack)

	return lgr, nil
}
