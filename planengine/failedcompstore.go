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
	ErrFailedCompensationNotFound = errors.New("failed compensation not found")
)

func (b *BadgerDB) StoreFailedCompensation(comp *FailedCompensation) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// Store the failed compensation data with full details
		compKey := fmt.Sprintf("failedcomp:info:%s", comp.ID)
		compData, err := json.Marshal(comp)
		if err != nil {
			return fmt.Errorf("failed to marshal failed compensation: %w", err)
		}

		if err := txn.Set([]byte(compKey), compData); err != nil {
			return fmt.Errorf("failed to store failed compensation: %w", err)
		}

		// Store project index (for retrieving by project)
		projectCompKey := fmt.Sprintf("failedcomp:project:%s:%s", comp.ProjectID, comp.ID)
		if err := txn.Set([]byte(projectCompKey), nil); err != nil {
			return fmt.Errorf("failed to store project compensation index: %w", err)
		}

		// Store orchestration index (for retrieving by orchestration)
		orchCompKey := fmt.Sprintf("failedcomp:orchestration:%s:%s", comp.OrchestrationID, comp.ID)
		if err := txn.Set([]byte(orchCompKey), nil); err != nil {
			return fmt.Errorf("failed to store orchestration compensation index: %w", err)
		}

		return nil
	})
}

func (b *BadgerDB) UpdateFailedCompensation(comp *FailedCompensation) error {
	// Check if compensation exists
	_, err := b.LoadFailedCompensation(comp.ID)
	if err != nil {
		return err
	}

	// Similar to store, but we know it already exists
	return b.db.Update(func(txn *badger.Txn) error {
		compKey := fmt.Sprintf("failedcomp:info:%s", comp.ID)
		compData, err := json.Marshal(comp)
		if err != nil {
			return fmt.Errorf("failed to marshal failed compensation: %w", err)
		}

		if err := txn.Set([]byte(compKey), compData); err != nil {
			return fmt.Errorf("failed to update failed compensation: %w", err)
		}

		return nil
	})
}

func (b *BadgerDB) LoadFailedCompensation(id string) (*FailedCompensation, error) {
	var comp FailedCompensation

	err := b.db.View(func(txn *badger.Txn) error {
		compKey := fmt.Sprintf("failedcomp:info:%s", id)
		item, err := txn.Get([]byte(compKey))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrFailedCompensationNotFound
			}
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &comp)
		})
	})

	if err != nil {
		return nil, err
	}

	return &comp, nil
}

func (b *BadgerDB) ListProjectFailedCompensations(projectID string) ([]*FailedCompensation, error) {
	var compensations []*FailedCompensation
	prefix := []byte(fmt.Sprintf("failedcomp:project:%s:", projectID))

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			compID := key[len(fmt.Sprintf("failedcomp:project:%s:", projectID)):]

			comp, err := b.LoadFailedCompensation(compID)
			if err != nil {
				return fmt.Errorf("failed to load compensation %s: %w", compID, err)
			}

			compensations = append(compensations, comp)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list project failed compensations: %w", err)
	}

	return compensations, nil
}

func (b *BadgerDB) ListOrchestrationFailedCompensations(orchestrationID string) ([]*FailedCompensation, error) {
	var compensations []*FailedCompensation
	prefix := []byte(fmt.Sprintf("failedcomp:orchestration:%s:", orchestrationID))

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			compID := key[len(fmt.Sprintf("failedcomp:orchestration:%s:", orchestrationID)):]

			comp, err := b.LoadFailedCompensation(compID)
			if err != nil {
				return fmt.Errorf("failed to load compensation %s: %w", compID, err)
			}

			compensations = append(compensations, comp)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list orchestration failed compensations: %w", err)
	}

	return compensations, nil
}

func (b *BadgerDB) ListFailedCompensations() ([]*FailedCompensation, error) {
	var compensations []*FailedCompensation
	prefix := []byte("failedcomp:info:")

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var comp FailedCompensation
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &comp)
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal failed compensation: %w", err)
			}
			compensations = append(compensations, &comp)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list failed compensations: %w", err)
	}

	return compensations, nil
}
