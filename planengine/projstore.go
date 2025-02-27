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
	"time"

	"github.com/dgraph-io/badger/v4"
)

var (
	ErrProjectNotFound       = errors.New("project not found")
	ErrProjectAPIKeyNotFound = errors.New("project api key not found")
)

func (b *BadgerDB) StoreProject(project *Project) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// Store project data
		projectKey := fmt.Sprintf("project:%s", project.ID)
		projectData, err := json.Marshal(project)
		if err != nil {
			return fmt.Errorf("failed to marshal project: %w", err)
		}

		if err := txn.Set([]byte(projectKey), projectData); err != nil {
			return fmt.Errorf("failed to store project: %w", err)
		}

		// Store API key index
		primaryKeyIndex := fmt.Sprintf("apikey:%s", project.APIKey)
		if err := txn.Set([]byte(primaryKeyIndex), []byte(project.ID)); err != nil {
			return fmt.Errorf("failed to store api key index: %w", err)
		}

		// Store additional API key indices
		for _, key := range project.AdditionalAPIKeys {
			additionalKeyIndex := fmt.Sprintf("apikey:%s", key)
			if err := txn.Set([]byte(additionalKeyIndex), []byte(project.ID)); err != nil {
				return fmt.Errorf("failed to store additional api key index: %w", err)
			}
		}

		return nil
	})
}

func (b *BadgerDB) LoadProject(id string) (*Project, error) {
	var project Project

	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("project:%s", id)))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrProjectNotFound
			}
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &project)
		})
	})

	if err != nil {
		return nil, err
	}

	return &project, nil
}

func (b *BadgerDB) LoadProjectByAPIKey(apiKey string) (*Project, error) {
	var projectID string

	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("apikey:%s", apiKey)))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrProjectAPIKeyNotFound
			}
			return err
		}

		return item.Value(func(val []byte) error {
			projectID = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return b.LoadProject(projectID)
}

func (b *BadgerDB) ListProjects() ([]*Project, error) {
	var projects []*Project

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("project:")

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var project Project

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &project)
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal project: %w", err)
			}

			projects = append(projects, &project)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	return projects, nil
}

func (b *BadgerDB) AddProjectAPIKey(projectID string, apiKey string) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// First load the project
		project, err := b.LoadProject(projectID)
		if err != nil {
			return err
		}

		// Add the new API key
		project.AdditionalAPIKeys = append(project.AdditionalAPIKeys, apiKey)
		project.UpdatedAt = time.Now().UTC()

		// Store the updated project
		projectData, err := json.Marshal(project)
		if err != nil {
			return fmt.Errorf("failed to marshal project: %w", err)
		}

		if err := txn.Set([]byte(fmt.Sprintf("project:%s", projectID)), projectData); err != nil {
			return fmt.Errorf("failed to update project: %w", err)
		}

		// Store the API key index
		if err := txn.Set([]byte(fmt.Sprintf("apikey:%s", apiKey)), []byte(projectID)); err != nil {
			return fmt.Errorf("failed to store api key index: %w", err)
		}

		return nil
	})
}

func (b *BadgerDB) AddProjectWebhook(projectID string, webhook string) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// First load the project
		project, err := b.LoadProject(projectID)
		if err != nil {
			return err
		}

		// Add the new webhook
		project.Webhooks = append(project.Webhooks, webhook)
		project.UpdatedAt = time.Now().UTC()

		// Store the updated project
		projectData, err := json.Marshal(project)
		if err != nil {
			return fmt.Errorf("failed to marshal project: %w", err)
		}

		return txn.Set([]byte(fmt.Sprintf("project:%s", projectID)), projectData)
	})
}
