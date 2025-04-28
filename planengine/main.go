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
	"github.com/posthog/posthog-go"

	"github.com/gorilla/mux"
)

func main() {
	postHogClient, _ := posthog.NewWithConfig("phc_oNzcBG9BiDfVaTE3gJTlCHTIwjBS68HLn4ZdKnkawoC", posthog.Config{Endpoint: "https://eu.i.posthog.com"})
	defer func(postHogClient posthog.Client) {
		_ = postHogClient.Close()
	}(postHogClient)

	cfg, err := Load()
	if err != nil {
		log.Fatalf("could not load plan engine config: %s", err.Error())
	}

	app, err := NewApp(cfg, os.Args)
	if err != nil {
		log.Fatalf("could not initialise plan engine server: %s", err.Error())
	}

	db, err := NewBadgerDB(cfg.StoragePath, app.Logger)
	if err != nil {
		log.Fatalf("could not initialise DB for plan engine server: %s", err.Error())
	}
	defer func(storage *BadgerDB) {
		_ = storage.Close()
	}(db)

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	llmClient, err := NewLLMClient(cfg, app.Logger)
	if err != nil {
		log.Fatalf("could not initialise LLM client for plan engine server: %s", err.Error())
	}

	engine := NewPlanEngine()
	telemetrySvc := NewTelemetryService(postHogClient, cfg.AnonymizedTelemetry, app.Logger)
	wsManager := NewWebSocketManager(app.Logger)
	matcher := NewMatcher(llmClient, app.Logger)
	vCache := NewVectorCache(llmClient, matcher, 1000, 24*time.Hour, app.Logger)
	logManager, err := NewLogManager(rootCtx, db, LogsRetentionPeriod, engine)
	if err != nil {
		log.Fatalf("could not initialise Log Manager for plan engine server: %s", err.Error())
	}
	pddlValidSvc := NewPddlValidationService(cfg.PddlValidatorPath, cfg.PddlValidationTimeout, app.Logger)
	logManager.Logger = app.Logger
	engine.Initialise(rootCtx, db, db, db, db, db, logManager, wsManager, vCache, pddlValidSvc, matcher, app.Logger, telemetrySvc)

	app.Engine = engine
	app.Router = mux.NewRouter()
	app.Db = db
	app.TelemetrySvc = telemetrySvc
	app.RootCtx = rootCtx
	app.RootCancel = rootCancel
	app.configureRoutes()
	app.configureWebSocket()
	app.Run()
}
