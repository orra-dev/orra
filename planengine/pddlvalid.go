/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

func NewPddlValidationService(valPath string, timeout time.Duration, logger zerolog.Logger) *PddlValidationService {
	return &PddlValidationService{
		valPath: valPath,
		timeout: timeout,
		logger:  logger,
	}
}

// Validate validates domain and problem PDDLs
func (s *PddlValidationService) Validate(ctx context.Context, projectID, domain, problem string) error {
	// Create temporary files
	domainPath, problemPath, cleanup, err := s.writePddlFiles(domain, problem)
	if err != nil {
		return fmt.Errorf("failed to write PDDL files: %w", err)
	}
	defer cleanup()

	s.logger.Trace().
		Str("ProjectID", projectID).
		Str("DomainPath", domainPath).
		Str("ProblemPath", problemPath).
		Msg("Validate with these paths")

	// Create command with timeout
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, s.valPath, domainPath, problemPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute validation
	if err := cmd.Run(); err != nil {
		// Handle different error types
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return &PddlValidationError{
				Type:    PddlTimeout,
				Message: "validation timed out",
			}
		}

		// Parse error output to determine type
		errStr := stderr.String()
		if strings.Contains(errStr, "syntax error") {
			return &PddlValidationError{
				Type:    PddlSyntax,
				Message: s.parseValidationError(errStr),
				Line:    s.extractLineNumber(errStr),
				File:    s.determineErrorFile(errStr),
			}
		}

		return &PddlValidationError{
			Type:    PddlProcess,
			Message: fmt.Sprintf("validation failed: %s", errStr),
		}
	}

	s.logger.Trace().
		Str("ProjectID", projectID).
		Str("DomainPath", domainPath).
		Str("ProblemPath", problemPath).
		Str("STDOUT", stdout.String()).
		Msg("Validated")

	return nil
}

func (s *PddlValidationService) HealthCheck(ctx context.Context) error {
	// Create simple test PDDL files
	const testDomain = `(define (domain test)
        (:requirements :strips)
        (:predicates (test))
        (:action dummy
            :parameters ()
            :precondition (test)
            :effect (not (test))))`

	const testProblem = `(define (problem test)
        (:domain test)
        (:init (test))
        (:goal (not (test))))`

	// Use shorter timeout for healthcheck
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	return s.Validate(ctx, "", testDomain, testProblem)
}

// writePddlFiles handles temporary file management with improved concurrency
func (s *PddlValidationService) writePddlFiles(domain, problem string) (domainPath, problemPath string, cleanup func(), err error) {
	// Create unique directory for this validation to prevent file collisions
	tmpDir, err := os.MkdirTemp("", "pddl-validate-*")
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Ensure directory cleanup
	cleanup = func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			s.logger.Error().Err(err).Str("path", tmpDir).Msg("Failed to cleanup temporary PDDL directory")
		}
	}

	// Create files within the unique directory
	domainPath = filepath.Join(tmpDir, "domain.pddl")
	problemPath = filepath.Join(tmpDir, "problem.pddl")

	// Write domain file with proper permissions
	if err := os.WriteFile(domainPath, []byte(domain), 0600); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("failed to write domain file: %w", err)
	}

	// Write problem file with proper permissions
	if err := os.WriteFile(problemPath, []byte(problem), 0600); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("failed to write problem file: %w", err)
	}

	return domainPath, problemPath, cleanup, nil
}

// Helper functions for error parsing
func (s *PddlValidationService) parseValidationError(errStr string) string {
	// Extract meaningful error message from VAL output
	// This needs to be customized based on VAL's error format
	lines := strings.Split(errStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "error:") {
			return strings.TrimSpace(strings.Split(line, "error:")[1])
		}
	}
	return errStr
}

func (s *PddlValidationService) extractLineNumber(errStr string) int {
	// Extract line number from error message
	r := regexp.MustCompile(`line (\d+)`)
	matches := r.FindStringSubmatch(errStr)
	if len(matches) > 1 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num
		}
	}
	return 0
}

func (s *PddlValidationService) determineErrorFile(errStr string) string {
	// Determine which file (domain/problem) has the error
	if strings.Contains(strings.ToLower(errStr), "domain") {
		return "domain"
	}
	if strings.Contains(strings.ToLower(errStr), "problem") {
		return "problem"
	}
	return ""
}

func (e *PddlValidationError) Error() string {
	location := ""
	if e.Line > 0 {
		location = fmt.Sprintf(" at line %d", e.Line)
	}
	if e.File != "" {
		location += fmt.Sprintf(" in %s file", e.File)
	}
	return fmt.Sprintf("%s error%s: %s", e.Type, location, e.Message)
}
