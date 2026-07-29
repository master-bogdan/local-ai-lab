package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxResponseBytes = 8 << 20

type Match struct {
	Path    string  `json:"path"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type Point struct {
	ID      string
	Path    string
	Content string
	Vector  []float64
}

type Store struct {
	ollamaURL  string
	qdrantURL  string
	model      string
	collection string
	client     *http.Client
}

func NewStore(ollamaURL, qdrantURL, model string, client *http.Client) *Store {
	return &Store{
		ollamaURL: strings.TrimRight(ollamaURL, "/"),
		qdrantURL: strings.TrimRight(qdrantURL, "/"),
		model:     model, collection: "local_ai_workspace", client: client,
	}
}

func (s *Store) Embed(ctx context.Context, text string) ([]float64, error) {
	request := map[string]any{"model": s.model, "input": text}
	var response struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := s.doJSON(ctx, http.MethodPost, s.ollamaURL+"/api/embed", request, &response); err != nil {
		return nil, fmt.Errorf("embed content: %w", err)
	}
	if len(response.Embeddings) != 1 || len(response.Embeddings[0]) == 0 {
		return nil, errors.New("no embedding returned by Ollama")
	}
	return response.Embeddings[0], nil
}

func (s *Store) Upsert(ctx context.Context, point Point) error {
	if err := s.ensureCollection(ctx, len(point.Vector)); err != nil {
		return err
	}
	payload := map[string]any{"points": []map[string]any{{
		"id": point.ID, "vector": point.Vector,
		"payload": map[string]string{"path": point.Path, "content": point.Content},
	}}}
	endpoint := fmt.Sprintf("%s/collections/%s/points?wait=true", s.qdrantURL, s.collection)
	if err := s.doJSON(ctx, http.MethodPut, endpoint, payload, nil); err != nil {
		return fmt.Errorf("upsert knowledge point: %w", err)
	}
	return nil
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]Match, error) {
	vector, err := s.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"query": vector, "limit": limit, "with_payload": true}
	var response struct {
		Result struct {
			Points []struct {
				Score   float64 `json:"score"`
				Payload struct {
					Path    string `json:"path"`
					Content string `json:"content"`
				} `json:"payload"`
			} `json:"points"`
		} `json:"result"`
	}
	endpoint := fmt.Sprintf("%s/collections/%s/points/query", s.qdrantURL, s.collection)
	if err := s.doJSON(ctx, http.MethodPost, endpoint, payload, &response); err != nil {
		return nil, fmt.Errorf("query knowledge: %w", err)
	}
	matches := make([]Match, 0, len(response.Result.Points))
	for _, point := range response.Result.Points {
		matches = append(matches, Match{Path: point.Payload.Path, Content: point.Payload.Content, Score: point.Score})
	}
	return matches, nil
}

func (s *Store) ensureCollection(ctx context.Context, vectorSize int) error {
	endpoint := fmt.Sprintf("%s/collections/%s", s.qdrantURL, s.collection)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("check Qdrant collection: %w", err)
	}
	response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return nil
	}
	if response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("check Qdrant collection: %s", response.Status)
	}
	payload := map[string]any{"vectors": map[string]any{"size": vectorSize, "distance": "Cosine"}}
	if err := s.doJSON(ctx, http.MethodPut, endpoint, payload, nil); err != nil {
		return fmt.Errorf("create Qdrant collection: %w", err)
	}
	return nil
}

func (s *Store) doJSON(ctx context.Context, method, endpoint string, payload, target any) error {
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", endpoint, response.Status)
	}
	if target == nil {
		return nil
	}
	responsePayload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(responsePayload) > maxResponseBytes {
		return fmt.Errorf("%s response exceeds %d bytes", endpoint, maxResponseBytes)
	}
	return json.Unmarshal(responsePayload, target)
}
