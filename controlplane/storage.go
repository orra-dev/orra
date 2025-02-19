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
	"github.com/rs/zerolog"
)

type BadgerLogStorage struct {
	db     *badger.DB
	logger zerolog.Logger
}

func NewBadgerLogStorage(dbPath string, logger zerolog.Logger) (*BadgerLogStorage, error) {
	opts := badger.DefaultOptions(dbPath)
	// Optimize for append-only workload
	opts.SyncWrites = true // Ensure durability
	opts.Logger = nil      // Use our own logger

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	return &BadgerLogStorage{
		db:     db,
		logger: logger,
	}, nil
}

func (b *BadgerLogStorage) Close() error {
	return b.db.Close()
}

func (b *BadgerLogStorage) Store(orchestrationID string, entry LogEntry) error {
	// Use orchestrationID in key for correct grouping and retrieval
	key := fmt.Sprintf("orchestration:%s:entry:%020d", orchestrationID, entry.GetOffset())
	value, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	})
}

func (b *BadgerLogStorage) StoreState(state *OrchestrationState) error {
	key := fmt.Sprintf("orchestration:%s:state", state.ID)
	value, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	})
}

func (b *BadgerLogStorage) LoadEntries(orchestrationID string) ([]LogEntry, error) {
	prefix := []byte(fmt.Sprintf("orchestration:%s:entry:", orchestrationID))
	var entries []LogEntry

	err := b.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var entry LogEntry
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &entry)
			})
			if err != nil {
				return err
			}
			entries = append(entries, entry)
		}
		return nil
	})

	return entries, err
}

func (b *BadgerLogStorage) ListOrchestrationStates() ([]*OrchestrationState, error) {
	prefix := []byte("orchestration:")
	var states []*OrchestrationState

	err := b.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := string(item.Key())

			// Only process state keys
			if !strings.HasSuffix(key, ":state") {
				continue
			}

			var state OrchestrationState
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &state)
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal state: %w", err)
			}
			states = append(states, &state)
		}
		return nil
	})

	return states, err
}

func (b *BadgerLogStorage) LoadState(orchestrationID string) (*OrchestrationState, error) {
	key := fmt.Sprintf("orchestration:%s:state", orchestrationID)
	var state OrchestrationState

	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return fmt.Errorf("orchestration state not found: %s", orchestrationID)
			}
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &state)
		})
	})

	if err != nil {
		return nil, err
	}

	return &state, nil
}
