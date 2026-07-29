package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	maxQueryBytes         = 4096
	maxSearXResponseBytes = 4 << 20
)

type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type Provider interface {
	Search(context.Context, string) ([]Result, error)
}

type Service struct {
	db       *sql.DB
	ttl      time.Duration
	provider Provider
}

func New(databasePath string, ttl time.Duration, provider Provider) (*Service, error) {
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open search cache: %w", err)
	}
	query := `CREATE TABLE IF NOT EXISTS search_cache (
		query TEXT PRIMARY KEY,
		payload BLOB NOT NULL,
		expires_at INTEGER NOT NULL
	)`
	if _, err := db.Exec(query); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize search cache: %w", err)
	}
	return &Service{db: db, ttl: ttl, provider: provider}, nil
}

func (s *Service) Search(ctx context.Context, query string) ([]Result, error) {
	if len(query) > maxQueryBytes {
		return nil, fmt.Errorf("search query exceeds %d bytes", maxQueryBytes)
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(query), " "))
	if normalized == "" {
		return nil, errors.New("search query is required")
	}
	results, found, err := s.cached(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if found {
		return results, nil
	}
	results, err = s.provider.Search(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("search provider: %w", err)
	}
	if err := s.store(ctx, normalized, results); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Service) Close() error {
	return s.db.Close()
}

func (s *Service) cached(ctx context.Context, query string) ([]Result, bool, error) {
	var payload []byte
	var expiresAt int64
	err := s.db.QueryRowContext(ctx, "SELECT payload, expires_at FROM search_cache WHERE query = ?", query).Scan(&payload, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read search cache: %w", err)
	}
	if expiresAt <= time.Now().Unix() {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM search_cache WHERE query = ?", query); err != nil {
			return nil, false, fmt.Errorf("expire search cache: %w", err)
		}
		return nil, false, nil
	}
	var results []Result
	if err := json.Unmarshal(payload, &results); err != nil {
		return nil, false, fmt.Errorf("decode search cache: %w", err)
	}
	return results, true, nil
}

func (s *Service) store(ctx context.Context, query string, results []Result) error {
	payload, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("encode search cache: %w", err)
	}
	expiresAt := time.Now().Add(s.ttl).Unix()
	statement := "INSERT OR REPLACE INTO search_cache(query, payload, expires_at) VALUES (?, ?, ?)"
	if _, err := s.db.ExecContext(ctx, statement, query, payload, expiresAt); err != nil {
		return fmt.Errorf("write search cache: %w", err)
	}
	return nil
}

type SearXNG struct {
	baseURL string
	client  *http.Client
}

func NewSearXNG(baseURL string, client *http.Client) SearXNG {
	return SearXNG{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

func (s SearXNG) Search(ctx context.Context, query string) ([]Result, error) {
	endpoint, err := url.Parse(s.baseURL + "/search")
	if err != nil {
		return nil, fmt.Errorf("parse SearXNG URL: %w", err)
	}
	parameters := endpoint.Query()
	parameters.Set("q", query)
	parameters.Set("format", "json")
	endpoint.RawQuery = parameters.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create SearXNG request: %w", err)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request SearXNG: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SearXNG returned %s", response.Status)
	}
	responsePayload, err := io.ReadAll(io.LimitReader(response.Body, maxSearXResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read SearXNG response: %w", err)
	}
	if len(responsePayload) > maxSearXResponseBytes {
		return nil, fmt.Errorf("SearXNG response exceeds %d bytes", maxSearXResponseBytes)
	}
	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(responsePayload, &payload); err != nil {
		return nil, fmt.Errorf("decode SearXNG response: %w", err)
	}
	results := make([]Result, 0, len(payload.Results))
	for _, result := range payload.Results {
		results = append(results, Result{Title: result.Title, URL: result.URL, Snippet: result.Content})
	}
	return results, nil
}
