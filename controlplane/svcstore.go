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
	ErrServiceNotFound = errors.New("service not found")
)

func (b *BadgerDB) StoreService(service *ServiceInfo) error {
	return b.db.Update(func(txn *badger.Txn) error {
		// Store service data
		serviceKey := fmt.Sprintf("service:info:%s", service.ID)
		serviceData, err := json.Marshal(service)
		if err != nil {
			return fmt.Errorf("failed to marshal service: %w", err)
		}

		if err := txn.Set([]byte(serviceKey), serviceData); err != nil {
			return fmt.Errorf("failed to store service: %w", err)
		}

		// Store project service index
		projectServiceKey := fmt.Sprintf("service:project:%s:%s", service.ProjectID, service.ID)
		if err := txn.Set([]byte(projectServiceKey), nil); err != nil {
			return fmt.Errorf("failed to store project service index: %w", err)
		}

		return nil
	})
}

func (b *BadgerDB) LoadServiceByProjectID(projectID, serviceID string) (*ServiceInfo, error) {
	var service ServiceInfo

	err := b.db.View(func(txn *badger.Txn) error {
		// First check if this service belongs to the project
		projectServiceKey := fmt.Sprintf("service:project:%s:%s", projectID, serviceID)
		_, err := txn.Get([]byte(projectServiceKey))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrServiceNotFound
			}
			return err
		}

		// Then load the service info
		item, err := txn.Get([]byte(fmt.Sprintf("service:info:%s", serviceID)))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrServiceNotFound
			}
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &service)
		})
	})

	if err != nil {
		return nil, err
	}

	// Doublecheck the loaded service matches the project
	if service.ProjectID != projectID {
		return nil, ErrServiceNotFound
	}

	return &service, nil
}

func (b *BadgerDB) LoadService(id string) (*ServiceInfo, error) {
	var service ServiceInfo

	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("service:info:%s", id)))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrServiceNotFound
			}
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &service)
		})
	})

	if err != nil {
		return nil, err
	}

	return &service, nil
}

func (b *BadgerDB) ListProjectServices(projectID string) ([]*ServiceInfo, error) {
	var services []*ServiceInfo
	prefix := []byte(fmt.Sprintf("service:project:%s:", projectID))

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			serviceID := key[len(fmt.Sprintf("service:project:%s:", projectID)):]

			service, err := b.LoadService(serviceID)
			if err != nil {
				return fmt.Errorf("failed to load service %s: %w", serviceID, err)
			}

			services = append(services, service)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list project services: %w", err)
	}

	return services, nil
}

func (b *BadgerDB) ListServices() ([]*ServiceInfo, error) {
	var services []*ServiceInfo
	prefix := []byte("service:info:")

	err := b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var service ServiceInfo
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &service)
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal service: %w", err)
			}
			services = append(services, &service)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	return services, nil
}
