/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"

	"github.com/rs/zerolog"
	"gonum.org/v1/gonum/mat"
)

type Embedder interface {
	CreateEmbeddings(ctx context.Context, text string) ([]float32, error)
}

type Matcher struct {
	embedder Embedder
	logger   zerolog.Logger
}

func NewMatcher(embedder Embedder, logger zerolog.Logger) *Matcher {
	return &Matcher{
		embedder: embedder,
		logger:   logger,
	}
}

func (m *Matcher) MatchTexts(ctx context.Context, text1, text2 string, threshold float64) (bool, float64, error) {
	// Generate embeddings for both texts
	vec1, err := m.GenerateEmbeddingVector(ctx, text1)
	if err != nil {
		return false, 0, err
	}

	vec2, err := m.GenerateEmbeddingVector(ctx, text2)
	if err != nil {
		return false, 0, err
	}

	// Calculate similarity
	similarity := CosineSimilarity(vec1, vec2)

	return similarity > threshold, similarity, nil
}

func (m *Matcher) GenerateEmbeddingVector(ctx context.Context, text string) (*mat.VecDense, error) {
	m.logger.Debug().Str("Text", text).Msg("generate embedding vector")
	embeddings, err := m.embedder.CreateEmbeddings(ctx, text)
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

// CosineSimilarity calculates cosine similarity between two vectors
func CosineSimilarity(a, b *mat.VecDense) float64 {
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

// NormalizeVector normalizes a vector to unit length
func NormalizeVector(v *mat.VecDense) {
	norm := mat.Norm(v, 2)
	if norm != 0 {
		v.ScaleVec(1/norm, v)
	}
}
