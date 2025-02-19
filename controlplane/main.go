/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"log"
	"os"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"github.com/gorilla/mux"
)

func main() {
	cfg, err := Load()
	if err != nil {
		log.Fatalf("could not load control plane config: %s", err.Error())
	}

	app, err := NewApp(cfg, os.Args)
	if err != nil {
		log.Fatalf("could not initialise control plane server: %s", err.Error())
	}

	storage, err := NewBadgerLogStorage(cfg.StoragePath, app.Logger)
	if err != nil {
		return
	}
	defer func(storage *BadgerLogStorage) {
		_ = storage.Close()
	}(storage)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	llmClient, err := NewLLMClient(cfg, app.Logger)
	if err != nil {
		log.Fatalf("could not initialise LLM client for control plane server: %s", err.Error())
	}

	plane := NewControlPlane()
	wsManager := NewWebSocketManager(app.Logger)
	matcher := NewMatcher(llmClient, app.Logger)
	vCache := NewVectorCache(llmClient, matcher, 1000, 24*time.Hour, app.Logger)
	logManager := NewLogManager(ctx, storage, LogsRetentionPeriod, plane)
	pddlValidSvc := NewPddlValidationService(cfg.PddlValidatorPath, cfg.PddlValidationTimeout, app.Logger)
	logManager.Logger = app.Logger
	plane.Initialise(ctx, logManager, wsManager, vCache, pddlValidSvc, matcher, app.Logger)

	app.Plane = plane
	app.Router = mux.NewRouter()
	app.configureRoutes()
	app.configureWebSocket()
	app.Run()
}
