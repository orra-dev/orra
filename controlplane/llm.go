/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/sashabaranov/go-openai"
)

const (
	groqAPIURLv1 = "https://api.groq.com/openai/v1"
	APITypeGroq  = "GROQ"
)

type LLMClient struct {
	reasoningClient  *openai.Client
	reasoningModel   string
	reasoningConfig  LLMClientConfig
	embeddingsClient *openai.Client
	embeddingsModel  string
	logger           zerolog.Logger
}

type LLMClientConfig struct {
	baseConfig          openai.ClientConfig
	maxCompletionTokens int
	temperature         float32
	topP                float32
	n                   int
}

func GroqConfig(authToken string) LLMClientConfig {
	cfg := openai.DefaultConfig(authToken)
	cfg.BaseURL = groqAPIURLv1
	cfg.APIType = APITypeGroq
	return LLMClientConfig{
		baseConfig:  cfg,
		temperature: 0.6,
		topP:        -1.0,
		n:           -1,
	}
}

func OpenAIConfig(authToken string) LLMClientConfig {
	cfg := openai.DefaultConfig(authToken)
	return LLMClientConfig{
		baseConfig:  cfg,
		temperature: 1.0,
		topP:        1.0,
		n:           1,
	}
}

func GetReasoningConfig(provider, apiKey string) (LLMClientConfig, error) {
	switch provider {
	case LLMGroqProvider:
		return GroqConfig(apiKey), nil
	case LLMOpenAIProvider:
		return OpenAIConfig(apiKey), nil
	default:
		return LLMClientConfig{}, fmt.Errorf("unknown LLM provider: %s", provider)
	}
}

func NewLLMClient(cfg Config, logger zerolog.Logger) (*LLMClient, error) {
	reasoningConfig, err := GetReasoningConfig(cfg.Reasoning.Provider, cfg.Reasoning.ApiKey)
	if err != nil {
		return nil, err
	}

	logger.Info().
		Str("ReasoningProvider", cfg.Reasoning.Provider).
		Str("ReasoningModel", cfg.Reasoning.Model).
		Msg("initialising LLM provider")

	return &LLMClient{
		reasoningClient:  openai.NewClientWithConfig(reasoningConfig.baseConfig),
		reasoningConfig:  reasoningConfig,
		reasoningModel:   cfg.Reasoning.Model,
		embeddingsClient: openai.NewClient(cfg.Embeddings.OpenaiApiKey),
		embeddingsModel:  string(openai.AdaEmbeddingV2),
		logger:           logger,
	}, nil
}

func (l *LLMClient) Generate(ctx context.Context, prompt string) (response string, err error) {
	l.logger.Trace().
		Str("Prompt", prompt).
		Msg("Decompose action prompt using cache powered completion")

	request := openai.ChatCompletionRequest{
		Model: l.reasoningModel,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
	}

	if l.reasoningConfig.temperature != -1.0 {
		request.Temperature = l.reasoningConfig.temperature
	}

	if l.reasoningConfig.topP != -1.0 {
		request.TopP = l.reasoningConfig.topP
	}

	if l.reasoningConfig.n != -1 {
		request.N = l.reasoningConfig.n
	}

	resp, err := l.reasoningClient.CreateChatCompletion(ctx, request)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

func (l *LLMClient) CreateEmbeddings(ctx context.Context, text string) ([]float32, error) {
	resp, err := l.embeddingsClient.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(l.embeddingsModel),
		Input: []string{text},
	})
	if err != nil {
		return nil, err
	}

	return resp.Data[0].Embedding, nil
}
