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

	"github.com/dgraph-io/badger/v4"
)

var (
	ErrOrchestrationNotFound = errors.New("orchestration not found")
)

// StoreOrchestration persists an orchestration
func (b *BadgerDB) StoreOrchestration(orchestration *Orchestration) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// Store orchestration data
		oKey := fmt.Sprintf("orchestration:info:%s", orchestration.ID)
		oData, err := json.Marshal(orchestration)
		if err != nil {
			return fmt.Errorf("failed to marshal orchestration: %w", err)
		}

		// Set orchestration data
		if err := txn.Set([]byte(oKey), oData); err != nil {
			return fmt.Errorf("failed to store orchestration: %w", err)
		}

		// Store project index for listing
		projectOrchestrationKey := fmt.Sprintf("orchestration:project:%s:%s",
			orchestration.ProjectID,
			orchestration.ID,
		)
		if err := txn.Set([]byte(projectOrchestrationKey), nil); err != nil {
			return fmt.Errorf("failed to store project orchestration index: %w", err)
		}

		return nil
	})
}

// LoadOrchestration retrieves an orchestration by its ID
func (b *BadgerDB) LoadOrchestration(id string) (*Orchestration, error) {
	var orchestration Orchestration

	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("orchestration:info:%s", id)))
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

func (b *BadgerDB) ListProjectOrchestrations(projectID string) ([]*Orchestration, error) {
	var orchestrations []*Orchestration
	prefix := []byte(fmt.Sprintf("orchestration:project:%s:", projectID))

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			oID := key[len(fmt.Sprintf("orchestration:project:%s:", projectID)):]

			orchestration, err := b.LoadOrchestration(oID)
			if err != nil {
				return fmt.Errorf("failed to load orchestration %s: %w", oID, err)
			}

			orchestrations = append(orchestrations, orchestration)
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
