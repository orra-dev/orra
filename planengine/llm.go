/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	open "github.com/sashabaranov/go-openai"
)

type Embedder interface {
	CreateEmbeddings(ctx context.Context, text string) ([]float32, error)
}

type LLMClient struct {
	llmClient        *open.Client
	llmModel         string
	embeddingsClient *open.Client
	embeddingsModel  string
	logger           zerolog.Logger
}

func createClientConfig(apiKey, apiBaseURL string) open.ClientConfig {
	cfg := open.DefaultConfig(apiKey)
	cfg.BaseURL = apiBaseURL
	return cfg
}

func getModelTemperature(model string) float32 {
	// Default temperature settings by model family
	switch {
	case strings.HasPrefix(model, O1MiniModel) || strings.HasPrefix(model, O3MiniModel):
		return 1.0 // OpenAI models
	case strings.HasPrefix(model, DeepseekR1Model):
		return 0.1 // Deepseek models
	case strings.HasPrefix(model, QwQ32BModel):
		return 0.1 // QwQ models
	default:
		return 0.7 // Default for unknown models
	}
}

func NewLLMClient(cfg Config, logger zerolog.Logger) (*LLMClient, error) {
	// Configure LLM client
	llmConfig := createClientConfig(cfg.LLM.ApiKey, cfg.LLM.ApiBaseURL)

	// Configure embeddings client
	embeddingsConfig := createClientConfig(cfg.Embeddings.ApiKey, cfg.Embeddings.ApiBaseURL)

	logger.Info().
		Str("LLMModel", cfg.LLM.Model).
		Str("LLMApiBaseURL", cfg.LLM.ApiBaseURL).
		Str("EmbeddingsModel", cfg.Embeddings.Model).
		Str("EmbeddingsApiBaseURL", cfg.Embeddings.ApiBaseURL).
		Msg("initialising AI providers")

	return &LLMClient{
		llmClient:        open.NewClientWithConfig(llmConfig),
		llmModel:         cfg.LLM.Model,
		embeddingsClient: open.NewClientWithConfig(embeddingsConfig),
		embeddingsModel:  cfg.Embeddings.Model,
		logger:           logger,
	}, nil
}

func (l *LLMClient) Generate(ctx context.Context, prompt string) (response string, err error) {
	l.logger.Trace().
		Str("Prompt", prompt).
		Msg("Decompose action prompt using cache powered completion")

	request := open.ChatCompletionRequest{
		Model: l.llmModel,
		Messages: []open.ChatCompletionMessage{
			{
				Role:    open.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Temperature: getModelTemperature(l.llmModel),
	}

	resp, err := l.llmClient.CreateChatCompletion(ctx, request)
	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "authentication") {
			return "", fmt.Errorf("authentication failed: API key may be required or invalid for this endpoint: %w", err)
		}
		return "", fmt.Errorf("LLM completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	return resp.Choices[0].Message.Content, nil
}

func (l *LLMClient) CreateEmbeddings(ctx context.Context, text string) ([]float32, error) {
	resp, err := l.embeddingsClient.CreateEmbeddings(ctx, open.EmbeddingRequest{
		Model: open.EmbeddingModel(l.embeddingsModel),
		Input: []string{text},
	})
	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "authentication") {
			return nil, fmt.Errorf("authentication failed: API key may be required or invalid for this endpoint: %w", err)
		}
		return nil, fmt.Errorf("embedding creation failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding model returned no data")
	}

	return resp.Data[0].Embedding, nil
}
