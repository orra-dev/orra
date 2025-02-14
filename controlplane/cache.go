/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"gonum.org/v1/gonum/mat"
)

func NewVectorCache(llmClient *LLMClient, maxSize int, ttl time.Duration, logger zerolog.Logger) *VectorCache {
	return &VectorCache{
		projectCaches: make(map[string]*ProjectCache),
		llmClient:     llmClient,
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

func (pc *ProjectCache) findBestMatch(query CacheQuery) (*CacheEntry, float64) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	var bestScore float64 = -1
	var bestEntry *CacheEntry

	// First pass: quick check on matching services
	for _, entry := range pc.entries {
		if entry.ServicesHash != query.servicesHash {
			continue
		}

		score := cosineSimilarity(query.actionVector, entry.ActionVector)

		pc.logger.Debug().
			Str("ActionWithFields", query.actionWithFields).
			Str("CachedActionWithFields To Match", entry.CachedActionWithFields).
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

func (c *VectorCache) Get(ctx context.Context, projectID, action string, actionParams json.RawMessage, serviceDescriptions string, grounding *GroundingSpec) (string, string, json.RawMessage, error) {
	result, err, _ := c.group.Do(fmt.Sprintf("%s:%s", projectID, action), func() (interface{}, error) {
		return c.getWithRetry(ctx, projectID, action, actionParams, serviceDescriptions, grounding)
	})

	if err != nil {
		return "", "", nil, err
	}

	cacheResult := result.(*CacheResult)

	if cacheResult.Hit && cacheResult.Task0Input != nil {
		modifiedResponse := cacheResult.Response
		newContent, err := substituteTask0Params(
			modifiedResponse,
			cacheResult.Task0Input,
			actionParams,
			cacheResult.CacheMappings,
		)
		if err != nil {
			return "", "", nil, err
		}
		modifiedResponse = newContent
		return modifiedResponse, cacheResult.ID, actionParams, nil
	}

	return cacheResult.Response, cacheResult.ID, actionParams, nil
}

func (c *VectorCache) getWithRetry(ctx context.Context, projectID, action string, rawActionParams json.RawMessage, serviceDescriptions string, grounding *GroundingSpec) (*CacheResult, error) {
	var actionParams ActionParams
	if err := json.Unmarshal(rawActionParams, &actionParams); err != nil {
		return nil, fmt.Errorf("failed to parse action params: %w", err)
	}
	servicesHash := computeHash(serviceDescriptions)

	c.logger.Trace().
		Str("serviceDescriptions", serviceDescriptions).
		Str("servicesHash", servicesHash).
		Msg("Hashed serviceDescriptions for cache evaluation")

	actionWithFields := fmt.Sprintf("%s:::%s", action, actionParams.String())
	actionVector, err := c.generateEmbeddingVector(ctx, actionWithFields)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for action with param fields: %w", err)
	}
	normalizeVector(actionVector)

	query := CacheQuery{
		actionWithFields: actionWithFields,
		actionParams:     actionParams,
		actionVector:     actionVector,
		servicesHash:     servicesHash,
	}
	if cached, cacheHit := c.lookupProjectCache(projectID, query); cacheHit {
		return cached, nil
	}

	c.logger.Debug().
		Str("projectID", projectID).
		Str("actionWithFields", actionWithFields).
		Msg("CACHE MISS")

	planJson, err := c.generateExecutionPlan(ctx, action, rawActionParams, serviceDescriptions, grounding)
	if err != nil {
		return nil, err
	}

	c.logger.Trace().RawJSON("Cache Miss Plan", []byte(planJson)).Msg("")

	task0Input, taskZeroCacheMappings, err := c.prepTaskZeroForCache(planJson, actionParams)
	if err != nil {
		return nil, err
	}

	cachedEntry := c.cache(projectID, planJson, actionVector, servicesHash, task0Input, taskZeroCacheMappings, actionWithFields)

	return &CacheResult{
		ID:         cachedEntry.ID,
		Response:   planJson,
		Task0Input: task0Input,
		Hit:        false,
	}, nil
}

func (c *VectorCache) lookupProjectCache(projectID string, query CacheQuery) (*CacheResult, bool) {
	pc := c.getProjectCache(projectID)
	bestEntry, bestScore := pc.findBestMatch(query)

	c.logger.Trace().
		Str("projectID", projectID).
		Str("actionWithFields", query.actionWithFields).
		Float64("bestScore", bestScore).
		Float64("threshold", pc.threshold).
		Msg("Cache lookup attempt")

	if bestEntry != nil && bestEntry.MatchesActionParams(query.actionParams) && bestScore > pc.threshold {
		if time.Since(bestEntry.Timestamp) < c.ttl {
			// Cache hit
			c.logger.Debug().
				Str("projectID", projectID).
				Str("actionWithFields", query.actionWithFields).
				Str("CachedActionWithFields", bestEntry.CachedActionWithFields).
				Float64("similarity", bestScore).
				Msg("CACHE HIT")

			return &CacheResult{
				ID:            bestEntry.ID,
				Response:      bestEntry.Response,
				Task0Input:    bestEntry.Task0Input,
				CacheMappings: bestEntry.CacheMappings,
				Hit:           true,
			}, true
		} else {
			c.Remove(projectID, bestEntry.ID)
		}
	}

	return nil, false
}

// generateExecutionPlan queries the LLM and extracts the execution plan.
func (c *VectorCache) generateExecutionPlan(ctx context.Context, action string, rawActionParams json.RawMessage, serviceDescriptions string, grounding *GroundingSpec) (string, error) {
	prompt := generatePlannerPrompt(action, rawActionParams, serviceDescriptions, grounding)
	llmResp, err := c.llmClient.Generate(ctx, prompt)
	if err != nil {
		return "", err
	}

	c.logger.Trace().Str("Raw llm Response", llmResp).Msg("")

	rawPlanJson, cot := cutCoT(llmResp)
	c.logger.Trace().Str("CoT", cot).Msg("If any")

	planJson, err := extractValidJSONOutput(rawPlanJson)
	if err != nil {
		return "", fmt.Errorf("cannot extract JSON from LLM response: %w", err)
	}

	return planJson, nil
}

// prepTaskZeroForCache prepares task zero so future cache hits can dynamically work with
// incoming action parameter values
func (c *VectorCache) prepTaskZeroForCache(execPlanJson string, actionParams ActionParams) (json.RawMessage, []TaskZeroCacheMapping, error) {

	task0Input, err := extractTask0Input(execPlanJson)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract task0 input: %w", err)
	}

	var task0InputMap map[string]interface{}
	if err := json.Unmarshal(task0Input, &task0InputMap); err != nil {
		return nil, nil, fmt.Errorf("failed to parse task0 input: %w", err)
	}
	c.logger.Trace().Interface("task0InputMap", task0InputMap).Msg("")

	// Extract parameter mappings
	mappings, err := extractParamMappings(actionParams, task0InputMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract parameter mappings: %w", err)
	}
	c.logger.Trace().Interface("Task0 Param Mapping", mappings).Msg("")

	return task0Input, mappings, nil
}

func (c *VectorCache) cache(projectID string, planJson string, actionVector *mat.VecDense, servicesHash string, task0Input json.RawMessage, taskZeroCacheMappings []TaskZeroCacheMapping, actionWithFields string) *CacheEntry {
	pc := c.getProjectCache(projectID)

	// Create new cache entry
	entry := &CacheEntry{
		ID:                     uuid.New().String(),
		Response:               planJson,
		ActionVector:           actionVector,
		ServicesHash:           servicesHash,
		Task0Input:             task0Input,
		CacheMappings:          taskZeroCacheMappings,
		Timestamp:              time.Now(),
		CachedActionWithFields: actionWithFields,
	}

	// Add to project cache
	pc.mu.Lock()
	if len(pc.entries) >= c.maxSize {
		// Remove oldest entry
		pc.entries = pc.entries[1:]
	}
	pc.entries = append(pc.entries, entry)
	pc.mu.Unlock()

	return entry
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
				Str("actionWithFields", entry.CachedActionWithFields).
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

func (c *VectorCache) generateEmbeddingVector(ctx context.Context, text string) (*mat.VecDense, error) {
	c.logger.Debug().Str("Text", text).Msg("generate embedding vector")
	embeddings, err := c.llmClient.CreateEmbeddings(ctx, text)
	if err != nil {
		return nil, err
	}

	// Convert to dense vector
	embeddingVector := mat.NewVecDense(len(embeddings), nil)
	for i, v := range embeddings {
		embeddingVector.SetVec(i, float64(v))
	}

	return embeddingVector, nil
}

func (p ActionParams) String() string {
	var builder strings.Builder
	for i, param := range p {
		builder.WriteString(param.Field)
		if i != len(p)-1 {
			builder.WriteString("::")
		}
	}
	return builder.String()
}

func (m TaskZeroCacheMappings) ContainsAll(params ActionParams) bool {
	for _, param := range params {
		if !m.IncludesActionField(param.Field) {
			return false
		}
	}
	return true
}

func (m TaskZeroCacheMappings) IncludesActionField(f string) bool {
	for _, mapping := range m {
		if mapping.ActionField == f {
			return true
		}
	}
	return false
}

func (c *CacheEntry) MatchesActionParams(actionParams ActionParams) bool {
	return c.CacheMappings.ContainsAll(actionParams)
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

func cutCoT(input string) (after string, cot string) {
	trimmed := strings.TrimSpace(input)

	afterTagStart, tagStart := strings.CutPrefix(trimmed, "<think>")
	if !tagStart {
		return input, ""
	}

	beforeTagEnd, afterTagEnd, tagEnd := strings.Cut(afterTagStart, "</think>")
	if !tagEnd {
		return input, ""
	}

	return afterTagEnd, beforeTagEnd
}
