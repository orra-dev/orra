package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/sashabaranov/go-openai"
	"gonum.org/v1/gonum/mat"
)

func NewVectorCache(openAIKey string, maxSize int, ttl time.Duration, logger zerolog.Logger) *VectorCache {
	return &VectorCache{
		projectCaches: make(map[string]*ProjectCache),
		embedder:      openai.NewClient(openAIKey),
		ttl:           ttl,
		maxSize:       maxSize,
		logger:        logger,
	}
}

func newProjectCache(logger zerolog.Logger) *ProjectCache {
	return &ProjectCache{
		entries:   make([]*CacheEntry, 0),
		threshold: 0.95, // High threshold for good matches
		logger:    logger,
	}
}

func computeHash(content string) string {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (pc *ProjectCache) findBestMatch(actionVector *mat.VecDense, servicesHash string, action string) (*CacheEntry, float64) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	var bestScore float64 = -1
	var bestEntry *CacheEntry

	// First pass: quick check on matching services
	for _, entry := range pc.entries {
		if entry.ServicesHash != servicesHash {
			continue
		}

		score := cosineSimilarity(actionVector, entry.ActionVector)

		pc.logger.Debug().
			Str("Action Cached", action).
			Str("Action To Match", entry.Action).
			Float64("score", score).
			Msg("cosineSimilarity")

		if score > bestScore {
			bestScore = score
			bestEntry = entry

			// Early exit on near-perfect match
			if score > 0.999 {
				break
			}
		}
	}

	return bestEntry, bestScore
}

func cosineSimilarity(a, b *mat.VecDense) float64 {
	if a.Len() != b.Len() {
		return -1
	}

	dotProduct := mat.Dot(a, b)
	normA := mat.Norm(a, 2)
	normB := mat.Norm(b, 2)

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (normA * normB)
}

func normalizeVector(v *mat.VecDense) {
	norm := mat.Norm(v, 2)
	if norm != 0 {
		v.ScaleVec(1/norm, v)
	}
}

func (c *VectorCache) getProjectCache(projectID string) *ProjectCache {
	c.mu.Lock()
	defer c.mu.Unlock()

	pc, exists := c.projectCaches[projectID]
	if !exists {
		pc = newProjectCache(c.logger)
		c.projectCaches[projectID] = pc
		c.logger.Info().
			Str("projectID", projectID).
			Msg("Created new project cache")
	}

	return pc
}

func (c *VectorCache) Get(
	ctx context.Context,
	projectID,
	action string,
	actionParams json.RawMessage,
	serviceDescriptions string,
) (*openai.ChatCompletionResponse, string, json.RawMessage, error) {
	result, err, _ := c.group.Do(fmt.Sprintf("%s:%s", projectID, action), func() (interface{}, error) {
		return c.getWithRetry(ctx, projectID, action, actionParams, serviceDescriptions)
	})

	if err != nil {
		return nil, "", nil, err
	}

	cacheResult := result.(*CacheResult)

	if cacheResult.Hit && cacheResult.Task0Input != nil {
		modifiedResponse := *cacheResult.Response
		newContent, err := substituteTask0Params(
			sanitizeJSONOutput(modifiedResponse.Choices[0].Message.Content),
			cacheResult.Task0Input,
			actionParams,
			cacheResult.ParamMappings,
		)
		if err != nil {
			return nil, "", nil, err
		}
		modifiedResponse.Choices[0].Message.Content = newContent
		return &modifiedResponse, cacheResult.ID, actionParams, nil
	}

	return cacheResult.Response, cacheResult.ID, actionParams, nil
}

func (c *VectorCache) getWithRetry(ctx context.Context,
	projectID,
	action string,
	actionParams json.RawMessage,
	serviceDescriptions string,
) (*CacheResult, error) {
	pc := c.getProjectCache(projectID)
	servicesHash := computeHash(serviceDescriptions)

	c.logger.Trace().
		Str("serviceDescriptions", serviceDescriptions).
		Str("servicesHash", servicesHash).
		Msg("Hashed serviceDescriptions for cache evaluation")

	// Generate embedding for the action
	actionEmbedding, err := c.generateEmbedding(ctx, action)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}
	normalizeVector(actionEmbedding)

	// Find best matching entry
	bestEntry, bestScore := pc.findBestMatch(actionEmbedding, servicesHash, action)

	c.logger.Trace().
		Str("projectID", projectID).
		Str("action", action).
		Float64("bestScore", bestScore).
		Float64("threshold", pc.threshold).
		Msg("Cache lookup attempt")

	// Cache hit
	if bestEntry != nil && bestScore > pc.threshold && time.Since(bestEntry.Timestamp) < c.ttl {
		c.logger.Debug().
			Str("projectID", projectID).
			Str("action", action).
			Float64("similarity", bestScore).
			Msg("CACHE HIT")

		return &CacheResult{
			ID:            bestEntry.ID,
			Response:      bestEntry.Response,
			Task0Input:    bestEntry.Task0Input,
			ParamMappings: bestEntry.ParamMappings,
			Hit:           true,
		}, nil
	}

	// Cache miss
	c.logger.Debug().
		Str("projectID", projectID).
		Str("action", action).
		Msg("CACHE MISS")

	llmResp, err := c.embedder.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4o,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: generateLLMPrompt(action, actionParams, serviceDescriptions),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	sanitizedAsJson := sanitizeJSONOutput(llmResp.Choices[0].Message.Content)
	c.logger.Debug().RawJSON("Cache Miss Plan", []byte(sanitizedAsJson)).Msg("")
	task0Input, err := extractTask0Input(sanitizedAsJson)
	if err != nil {
		return nil, fmt.Errorf("failed to extract task0 input: %w", err)
	}

	// Parse action params and task0 input for mapping
	var params ActionParams
	if err := json.Unmarshal(actionParams, &params); err != nil {
		return nil, fmt.Errorf("failed to parse action params: %w", err)
	}

	var task0InputMap map[string]interface{}
	if err := json.Unmarshal(task0Input, &task0InputMap); err != nil {
		return nil, fmt.Errorf("failed to parse task0 input: %w", err)
	}

	// Extract parameter mappings
	mappings, err := extractParamMappings(params, task0InputMap)
	if err != nil {
		return nil, fmt.Errorf("failed to extract parameter mappings: %w", err)
	}

	// Create new cache entry
	entry := &CacheEntry{
		ID:            uuid.New().String(),
		Response:      &llmResp,
		ActionVector:  actionEmbedding,
		ServicesHash:  servicesHash,
		Task0Input:    task0Input,
		ParamMappings: mappings,
		Timestamp:     time.Now(),
		Action:        action,
	}

	// Add to project cache
	pc.mu.Lock()
	if len(pc.entries) >= c.maxSize {
		// Remove oldest entry
		pc.entries = pc.entries[1:]
	}
	pc.entries = append(pc.entries, entry)
	pc.mu.Unlock()

	return &CacheResult{
		ID:         entry.ID,
		Response:   &llmResp,
		Task0Input: task0Input,
		Hit:        false,
	}, nil
}

func (c *VectorCache) Remove(projectID, id string) bool {
	c.mu.RLock()
	pc, exists := c.projectCaches[projectID]
	c.mu.RUnlock()

	if !exists {
		return false
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	for i, entry := range pc.entries {
		if entry.ID == id {
			pc.entries = append(pc.entries[:i], pc.entries[i+1:]...)
			c.logger.Debug().
				Str("projectID", projectID).
				Str("id", id).
				Str("action", entry.Action).
				Msg("Removed cache entry")
			return true
		}
	}
	return false
}

func (c *VectorCache) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(c.ttl / 2)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.cleanup()
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func (c *VectorCache) cleanup() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	for projectID, pc := range c.projectCaches {
		pc.mu.Lock()
		var validIdx int
		for i, entry := range pc.entries {
			if now.Sub(entry.Timestamp) < c.ttl {
				if validIdx != i {
					pc.entries[validIdx] = entry
				}
				validIdx++
			}
		}
		pc.entries = pc.entries[:validIdx]
		pc.mu.Unlock()

		c.logger.Debug().
			Str("projectID", projectID).
			Int("remainingEntries", validIdx).
			Msg("Cleaned project cache")
	}
}

func (c *VectorCache) generateEmbedding(ctx context.Context, text string) (*mat.VecDense, error) {
	c.logger.Debug().Str("Input", text).Msg("generate embedding for input")
	resp, err := c.embedder.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.AdaEmbeddingV2,
		Input: []string{text},
	})
	if err != nil {
		return nil, err
	}

	// Convert to dense vector
	embedding := mat.NewVecDense(len(resp.Data[0].Embedding), nil)
	for i, v := range resp.Data[0].Embedding {
		embedding.SetVec(i, float64(v))
	}

	return embedding, nil
}

// extractTask0Input extracts the input parameters from task0 in the calling plan
func extractTask0Input(content string) (json.RawMessage, error) {
	var plan ServiceCallingPlan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse calling plan: %w", err)
	}

	// Find task0
	for _, task := range plan.Tasks {
		if task.ID == "task0" {
			// Marshal the input map to get the exact JSON structure
			input, err := json.Marshal(task.Input)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal task0 input: %w", err)
			}
			return input, nil
		}
	}

	return nil, fmt.Errorf("task0 not found in calling plan")
}
