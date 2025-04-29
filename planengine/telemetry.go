/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/posthog/posthog-go"
	"github.com/rs/zerolog"
)

// TelemetryService provides functionality for anonymous usage tracking
// with the ability to opt-out via environment variable.
type TelemetryService struct {
	// Client is the PostHog client used to track events
	client posthog.Client

	// Logger for telemetry operations
	logger zerolog.Logger

	// Whether telemetry is enabled (based on the ANONYMIZED_TELEMETRY env var)
	enabled bool
}

// NewTelemetryService creates a new telemetry service.
// It respects the ANONYMIZED_TELEMETRY environment variable setting.
// If ANONYMIZED_TELEMETRY is set to false, telemetry will be disabled.
func NewTelemetryService(client posthog.Client, enabled bool, logger zerolog.Logger) *TelemetryService {
	return &TelemetryService{
		client:  client,
		enabled: enabled,
		logger:  logger.With().Str("component", "telemetry").Logger(),
	}
}

// TrackEvent tracks an event in PostHog if telemetry is enabled.
// The event will be ignored if telemetry is disabled.
func (s *TelemetryService) TrackEvent(event string, properties map[string]any) {
	if !s.enabled {
		return
	}

	// Use anonymous ID for privacy
	anonymousID := getOrCreateAnonymousID()

	// Remove $ip property if present
	delete(properties, "$ip")

	// Add event to PostHog
	err := s.client.Enqueue(posthog.Capture{
		DistinctId: anonymousID,
		Event:      event,
		Properties: properties,
	})

	if err != nil {
		s.logger.Error().Err(err).Str("event", event).Msg("Failed to track telemetry event")
	} else {
		s.logger.Debug().Str("event", event).Msg("Telemetry event tracked")
	}
}

// IsEnabled returns whether telemetry is enabled
func (s *TelemetryService) IsEnabled() bool {
	return s.enabled
}

func getOrCreateAnonymousID() string {
	tempDir := os.TempDir()
	uuidFile := filepath.Join(tempDir, AnonymouseIDFilename)

	if data, err := os.ReadFile(uuidFile); err == nil {
		return string(data)
	}

	id := uuid.New().String()
	_ = os.WriteFile(uuidFile, []byte(id), 0600)
	return id
}

func HashUUID(uuidStr string) string {
	hash := sha256.Sum256([]byte(uuidStr))
	return hex.EncodeToString(hash[:])
}
