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

	db, err := NewBadgerDB(cfg.StoragePath, app.Logger)
	if err != nil {
		log.Fatalf("could not initialise DB for control plane server: %s", err.Error())
	}
	defer func(storage *BadgerDB) {
		_ = storage.Close()
	}(db)

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
	logManager, err := NewLogManager(ctx, db, LogsRetentionPeriod, plane)
	if err != nil {
		log.Fatalf("could not initialise Log Manager for control plane server: %s", err.Error())
	}
	pddlValidSvc := NewPddlValidationService(cfg.PddlValidatorPath, cfg.PddlValidationTimeout, app.Logger)
	logManager.Logger = app.Logger
	plane.Initialise(ctx, db, logManager, wsManager, vCache, pddlValidSvc, matcher, app.Logger)

	app.Plane = plane
	app.Router = mux.NewRouter()
	app.Db = db
	app.configureRoutes()
	app.configureWebSocket()
	app.Run()
}
