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

	cmd.AddCommand(newVerifyTellCmd(opts))
	cmd.AddCommand(newVerifyWebhooksCmd(opts))

	return cmd
}

func newVerifyTellCmd(opts *CliOpts) *cobra.Command {
	var (
		data       []string
		webhookUrl string
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:   "tell [action]",
		Short: "Tell Orra to orchestrate an action with data parameters",
		Long:  "Tell Orra to orchestrate an action with at least one data parameter",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action := args[0]

			proj, projectName, err := config.GetProject(opts.Config, opts.ProjectID)
			if err != nil {
				return err
			}

			if len(webhookUrl) == 0 {
				webhookUrl = proj.Webhooks[0]
			}

			if !contains(proj.Webhooks, webhookUrl) {
				return fmt.Errorf("unknown webhook for project %s", projectName)
			}

			actionParams, err := convertToActionParams(data)
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
				Data:    actionParams,
				Webhook: webhookUrl,
			})
			if err != nil {
				return fmt.Errorf("failed to create orchestration: %w", err)
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
			fmt.Printf("3. Orchestrating available services\n\n")

			fmt.Printf("Available commands:\n")
			fmt.Printf("- View progress:                orra inspect %s\n", orchestration.ID)
			fmt.Printf("- View full progress details:   orra inspect -d %s\n", orchestration.ID)
			fmt.Printf("- List all orchestrations:      orra ps\n")

			fmt.Printf("Tip: Keep an eye on your webhook server to see the results!\n")
			return nil
		},
	}

	cmd.Flags().StringSliceVarP(&data, "data", "d", []string{}, "Data to supplement action in format param:value")
	cmd.Flags().StringVarP(&webhookUrl, "webhook", "w", "", "Webhook url (defaults to first configured webhook)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress extra explanation (defaults to false)")

	return cmd
}

func convertToActionParams(params []string) ([]map[string]string, error) {
	var actionParams []map[string]string
	for _, param := range params {
		parts := strings.SplitN(param, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid parameter format: %s (should be name:value)", param)
		}
		actionParams = append(actionParams, map[string]string{
			"field": parts[0],
			"value": parts[1],
		})
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
	//cmd.AddCommand(newVerifyWebhooksStopCmd(opts))

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
				return fmt.Errorf("unknown webhook %s for project: %s", webhookURL, projectName)
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

//func newVerifyWebhooksStopCmd(opts *CliOpts) *cobra.Command {
//	return &cobra.Command{
//		Use:   "stop",
//		Short: "Stop all test webhook servers",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			webhooksMutex.Lock()
//			urls := make([]string, 0, len(runningWebhooks))
//			for url := range runningWebhooks {
//				urls = append(urls, url)
//			}
//			webhooksMutex.Unlock()
//
//			for _, url := range urls {
//				if err := stopWebhook(url); err != nil {
//					return err
//				}
//			}
//			return nil
//		},
//	}
//}

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
