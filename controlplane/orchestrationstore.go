/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

var (
	ErrOrchestrationNotFound = errors.New("orchestration not found")
)

// StoreOrchestration persists an orchestration to BadgerDB
func (b *BadgerDB) StoreOrchestration(orchestration *Orchestration) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// Store orchestration data
		orchKey := fmt.Sprintf("orchestration:%s", orchestration.ID)
		orchData, err := json.Marshal(orchestration)
		if err != nil {
			return fmt.Errorf("failed to marshal orchestration: %w", err)
		}

		// Set orchestration data
		if err := txn.Set([]byte(orchKey), orchData); err != nil {
			return fmt.Errorf("failed to store orchestration: %w", err)
		}

		// Store project index for listing
		projectIndexKey := fmt.Sprintf("project-orchestrations:%s:%s:%d",
			orchestration.ProjectID,
			orchestration.ID,
			orchestration.Timestamp.UnixNano(),
		)
		if err := txn.Set([]byte(projectIndexKey), nil); err != nil {
			return fmt.Errorf("failed to store project orchestration index: %w", err)
		}

		return nil
	})
}

// LoadOrchestration retrieves an orchestration by its ID
func (b *BadgerDB) LoadOrchestration(id string) (*Orchestration, error) {
	var orchestration Orchestration

	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("orchestration:%s", id)))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrOrchestrationNotFound
			}
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &orchestration)
		})
	})

	if err != nil {
		return nil, err
	}

	return &orchestration, nil
}

func (b *BadgerDB) ListOrchestrations(projectID string) ([]*Orchestration, error) {
	var orchestrations []*Orchestration

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(fmt.Sprintf("project-orchestrations:%s:", projectID))
		opts.Reverse = true // Get newest first

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			// Extract orchestration ID from the index key
			key := string(it.Item().Key())
			orchID := extractOrchestrationIDFromIndex(key)
			if orchID == "" {
				continue
			}

			// Load the actual orchestration
			item, err := txn.Get([]byte(fmt.Sprintf("orchestration:%s", orchID)))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					// Skip if orchestration was deleted
					continue
				}
				b.logger.Error().
					Err(err).
					Str("OrchestrationID", orchID).
					Msg("Failed to load orchestration")
				continue
			}

			var orch Orchestration
			err = item.Value(func(val []byte) error {
				return json.Unmarshal(val, &orch)
			})
			if err != nil {
				b.logger.Error().
					Err(err).
					Str("OrchestrationID", orchID).
					Msg("Failed to unmarshal orchestration")
				continue
			}

			orchestrations = append(orchestrations, &orch)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list orchestrations: %w", err)
	}

	// Return empty slice rather than nil if no orchestrations found
	if orchestrations == nil {
		orchestrations = make([]*Orchestration, 0)
	}

	return orchestrations, nil
}

// Helper function to extract orchestration ID from index key
func extractOrchestrationIDFromIndex(key string) string {
	// Format: project-orchestrations:<projectID>:<orchID>:<timestamp>
	parts := strings.Split(key, ":")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}
