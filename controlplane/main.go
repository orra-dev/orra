package main

import (
	"context"
	"log"
	"os"

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

	plane := NewControlPlane(cfg.OpenApiKey)
	wsManager := NewWebSocketManager(app.Logger)
	logManager := NewLogManager(ctx, LogsRetentionPeriod, plane)
	logManager.Logger = app.Logger
	plane.Initialise(ctx, logManager, wsManager, app.Logger)

	app.Plane = plane
	app.Router = mux.NewRouter()
	app.configureRoutes()
	app.configureWebSocket()
	app.Run()
}
