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
	ErrGroundingNotFound = errors.New("grounding not found")
)

func (b *BadgerDB) StoreGrounding(grounding *GroundingSpec) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// Store grounding data
		groundingKey := fmt.Sprintf("grounding:info:%s:%s", grounding.ProjectID, grounding.Name)
		groundingData, err := json.Marshal(grounding)
		if err != nil {
			return fmt.Errorf("failed to marshal grounding: %w", err)
		}

		if err := txn.Set([]byte(groundingKey), groundingData); err != nil {
			return fmt.Errorf("failed to store grounding: %w", err)
		}

		// Store project grounding index
		projectGroundingKey := fmt.Sprintf("grounding:project:%s:%s", grounding.ProjectID, grounding.Name)
		if err := txn.Set([]byte(projectGroundingKey), nil); err != nil {
			return fmt.Errorf("failed to store project grounding index: %w", err)
		}

		return nil
	})
}

func (b *BadgerDB) LoadGrounding(projectID, name string) (*GroundingSpec, error) {
	var grounding GroundingSpec

	err := b.db.View(func(txn *badger.Txn) error {
		// First check if this grounding belongs to the project
		projectGroundingKey := fmt.Sprintf("grounding:project:%s:%s", projectID, name)
		_, err := txn.Get([]byte(projectGroundingKey))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrGroundingNotFound
			}
			return err
		}

		// Then load the grounding info
		groundingKey := fmt.Sprintf("grounding:info:%s:%s", projectID, name)
		item, err := txn.Get([]byte(groundingKey))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrGroundingNotFound
			}
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &grounding)
		})
	})

	if err != nil {
		return nil, err
	}

	return &grounding, nil
}

func (b *BadgerDB) ListProjectGroundings(projectID string) ([]*GroundingSpec, error) {
	var groundings []*GroundingSpec
	prefix := []byte(fmt.Sprintf("grounding:project:%s:", projectID))

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			name := key[len(fmt.Sprintf("grounding:project:%s:", projectID)):]

			grounding, err := b.LoadGrounding(projectID, name)
			if err != nil {
				return fmt.Errorf("failed to load grounding %s: %w", name, err)
			}

			groundings = append(groundings, grounding)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list project groundings: %w", err)
	}

	return groundings, nil
}

func (b *BadgerDB) ListGroundings() ([]*GroundingSpec, error) {
	var groundings []*GroundingSpec
	prefix := []byte("grounding:info:")

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var grounding GroundingSpec
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &grounding)
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal grounding: %w", err)
			}
			groundings = append(groundings, &grounding)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list groundings: %w", err)
	}

	return groundings, nil
}
