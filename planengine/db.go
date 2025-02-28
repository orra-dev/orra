/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog"
)

type BadgerDB struct {
	db     *badger.DB
	logger zerolog.Logger
}

func NewBadgerDB(dbPath string, logger zerolog.Logger) (*BadgerDB, error) {
	opts := badger.DefaultOptions(dbPath)
	// Optimize for append-only workload
	opts.SyncWrites = true // Ensure durability
	opts.Logger = nil      // Use our own logger

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open DB: %w", err)
	}

	logger.Info().Msgf("Started DB at: %s", dbPath)

	return &BadgerDB{
		db:     db,
		logger: logger,
	}, nil
}

func (b *BadgerDB) Close() error {
	return b.db.Close()
}
