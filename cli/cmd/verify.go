/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ezodude/orra/cli/internal/api"
	"github.com/ezodude/orra/cli/internal/config"
	"github.com/spf13/cobra"
)

func newVerifyCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify your Orra setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newVerifyRunCmd(opts))
	cmd.AddCommand(newVerifyWebhooksCmd(opts))

	return cmd
}

func newVerifyRunCmd(opts *CliOpts) *cobra.Command {
	var (
		data                   []string
		timeout                string
		healthCheckGracePeriod string
		quiet                  bool
	)

	cmd := &cobra.Command{
		Use:   "run [action]",
		Short: "Orchestrate an action with any data parameters",
		Long: `Orchestrate an action with any data parameters to dynamically 
execute the required project services`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action := args[0]

			proj, projectName, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			actionParams, err := parseActionParamsJSON(data)
			if err != nil {
				return err
			}

			client := opts.ApiClient.
				SetBaseUrl(proj.ServerAddr).
				SetApiKey(proj.CliAuth)

			ctx, cancel := context.WithTimeout(cmd.Context(), client.GetTimeout())
			defer cancel()

			orchestration, err := client.CreateOrchestration(ctx, api.OrchestrationRequest{
				Action: struct{ Content string }{
					Content: action,
				},
				Data:                   actionParams,
				Timeout:                timeout,
				HealthCheckGracePeriod: healthCheckGracePeriod,
			})
			if err != nil {
				return fmt.Errorf("failed to create orchestration - %w", err)
			}

			fmt.Printf("Project: %s\nServer:  %s\n", projectName, proj.ServerAddr)
			fmt.Printf("Action Orchestrated!\n")
			fmt.Printf("Action: %s\n", action)
			fmt.Printf("Created orchestration: %s\n", orchestration.ID)
			fmt.Printf("Inspect progress with: orra inspect %s\n", orchestration.ID)

			if quiet {
				return nil
			}

			fmt.Printf("\nWhat's happening:\n")
			fmt.Printf("1. Orra is analyzing your action using AI\n")
			fmt.Printf("2. Creating an execution plan\n")
			fmt.Printf("3. Orchestrating available services and/or agents\n\n")

			fmt.Printf("Available commands:\n")
			fmt.Printf("- View progress:                orra inspect %s\n", orchestration.ID)
			fmt.Printf("- View full progress details:   orra inspect -d %s\n", orchestration.ID)
			fmt.Printf("- List all orchestrations:      orra ps\n")

			fmt.Printf("Tip: Keep an eye on your webhook server to see the results!\n")
			return nil
		},
	}

	//cmd.Flags().StringSliceVarP(&data, "data", "d", []string{}, "Data to supplement action in format param:value")
	cmd.Flags().StringArrayVarP(&data, "data", "d", []string{}, "Data to supplement action in format param:value or param:json")
	cmd.Flags().StringVarP(&timeout, "timeout", "t", "", `Set execution timeout duration per service/agent
(defaults to 30s)`)
	cmd.Flags().StringVarP(&healthCheckGracePeriod, "health-check-grace-period", "g", "", `Set grace period for an unhealthy service or agent before terminating an orchestration
(defaults to 30m)`)
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, `Suppress extra explanation
(defaults to false)`)

	return cmd
}

func parseActionParamsJSON(params []string) ([]map[string]interface{}, error) {
	var actionParams []map[string]interface{}

	for _, param := range params {
		parts := strings.SplitN(param, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid parameter format: %s (should be name:value)", param)
		}

		field := parts[0]
		valueStr := parts[1]

		// Check for array-like format with commas but no brackets (easier CLI input)
		if strings.Contains(valueStr, ",") && !strings.HasPrefix(valueStr, "[") {
			// Split by comma and create a string array
			values := strings.Split(valueStr, ",")
			// Trim spaces
			for i, v := range values {
				values[i] = strings.TrimSpace(v)
			}
			actionParams = append(actionParams, map[string]interface{}{
				"field": field,
				"value": values, // This will be serialized as a JSON array
			})
		} else if (strings.HasPrefix(valueStr, "[") && strings.HasSuffix(valueStr, "]")) ||
			(strings.HasPrefix(valueStr, "{") && strings.HasSuffix(valueStr, "}")) {
			// Standard JSON parsing
			var jsonValue interface{}
			if err := json.Unmarshal([]byte(valueStr), &jsonValue); err != nil {
				return nil, fmt.Errorf("invalid JSON for parameter %s: %w", field, err)
			}
			actionParams = append(actionParams, map[string]interface{}{
				"field": field,
				"value": jsonValue,
			})
		} else {
			// Regular string value
			actionParams = append(actionParams, map[string]interface{}{
				"field": field,
				"value": valueStr,
			})
		}
	}

	return actionParams, nil
}

func newVerifyWebhooksCmd(opts *CliOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhooks",
		Short: "Manage verify webhooks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newVerifyWebhooksStartCmd(opts))

	return cmd
}

// WebhookServer maintains state of running verify webhooks
type WebhookServer struct {
	server *http.Server
	done   chan bool
}

var (
	runningWebhooks = make(map[string]*WebhookServer)
	webhooksMutex   sync.Mutex
)

func newVerifyWebhooksStartCmd(opts *CliOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "start [webhook-url]",
		Short: "Start a verify webhook server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			webhookURL := args[0]

			proj, projectName, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			if !contains(proj.Webhooks, webhookURL) {
				if !contains(proj.Webhooks, strings.ReplaceAll(webhookURL, "localhost", "host.docker.internal")) {
					return fmt.Errorf("unknown webhook %s for project: %s", webhookURL, projectName)
				}
			}

			webhooksMutex.Lock()
			if _, exists := runningWebhooks[webhookURL]; exists {
				webhooksMutex.Unlock()
				return fmt.Errorf("webhook server already running for %s for project: %s", webhookURL, projectName)
			}
			webhooksMutex.Unlock()

			// Parse webhook URL
			parsedURL, err := url.Parse(webhookURL)
			if err != nil {
				return fmt.Errorf("invalid webhook URL: %w", err)
			}

			port := "8888" // Default port
			if parsedURL.Port() != "" {
				port = parsedURL.Port()
			}

			// Get the path, use "/" if none specified
			path := parsedURL.Path
			if path == "" {
				path = "/"
			}

			// Create server
			mux := http.NewServeMux()
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				// Only handle POST requests
				if r.Method != http.MethodPost {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
					return
				}

				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					fmt.Printf("Error decoding webhook payload: %v\n", err)
					http.Error(w, "Bad request", http.StatusBadRequest)
					return
				}

				prettyPayload, _ := json.MarshalIndent(payload, "", "  ")
				fmt.Printf("\nReceived webhook payload on %s:\n%s\n", path, string(prettyPayload))

				// Return success response
				w.WriteHeader(http.StatusOK)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "received"})
			})

			server := &http.Server{
				Addr:    ":" + port,
				Handler: mux,
			}

			ws := &WebhookServer{
				server: server,
				done:   make(chan bool),
			}

			webhooksMutex.Lock()
			runningWebhooks[webhookURL] = ws
			webhooksMutex.Unlock()

			// Start server in background
			go func() {
				fmt.Printf("Starting webhook server on http://localhost:%s%s\n", port, path)
				if err := server.ListenAndServe(); err != http.ErrServerClosed {
					fmt.Printf("Webhook server error: %v\n", err)
				}
				close(ws.done)
			}()

			// Handle graceful shutdown
			signalChan := make(chan os.Signal, 1)
			signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

			<-signalChan
			fmt.Println("\nReceived shutdown signal")
			if err := stopWebhook(webhookURL); err != nil {
				fmt.Printf("Error stopping webhook: %v\n", err)
			}

			return nil
		},
	}
}

func stopWebhook(url string) error {
	webhooksMutex.Lock()
	ws, exists := runningWebhooks[url]
	webhooksMutex.Unlock()

	if !exists {
		return fmt.Errorf("no webhook server running for %s", url)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := ws.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("error shutting down webhook server: %w", err)
	}

	<-ws.done // Wait for server to finish

	webhooksMutex.Lock()
	delete(runningWebhooks, url)
	webhooksMutex.Unlock()

	fmt.Printf("Stopped webhook server for %s\n", url)
	return nil
}
