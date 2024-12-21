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
		log.Fatalf("could not load api config: %s", err.Error())
	}

	app, err := NewApp(cfg, os.Args)
	if err != nil {
		log.Fatalf("could not initialise control plane server: %s", err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	plane := NewControlPlane(cfg.OpenaiApiKey)
	wsManager := NewWebSocketManager(app.Logger)
	vCache := NewVectorCache(cfg.OpenaiApiKey, 1000, 24*time.Hour, app.Logger)
	logManager := NewLogManager(ctx, LogsRetentionPeriod, plane)
	logManager.Logger = app.Logger
	plane.Initialise(ctx, logManager, wsManager, vCache, app.Logger)

	app.Plane = plane
	app.Router = mux.NewRouter()
	app.configureRoutes()
	app.configureWebSocket()
	app.Run()
}
