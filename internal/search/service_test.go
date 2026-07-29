package search_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/search"
)

type searchProvider struct {
	calls int
}

func (p *searchProvider) Search(_ context.Context, _ string) ([]search.Result, error) {
	p.calls++
	return []search.Result{{Title: "Go", URL: "https://go.dev", Snippet: "Go documentation"}}, nil
}

func TestServiceCachesSearchResultsInSQLite(t *testing.T) {
	provider := &searchProvider{}
	service, err := search.New(filepath.Join(t.TempDir(), "search.db"), time.Hour, provider)
	if err != nil {
		t.Fatalf("create search service: %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })

	first, err := service.Search(context.Background(), " Go documentation ")
	if err != nil {
		t.Fatalf("first search: %v", err)
	}
	second, err := service.Search(context.Background(), "go documentation")
	if err != nil {
		t.Fatalf("cached search: %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf("expected one provider request, got %d", provider.calls)
	}
	if len(first) != 1 || len(second) != 1 || second[0].URL != first[0].URL {
		t.Fatalf("cached response differs: first=%#v second=%#v", first, second)
	}
}

func TestCacheReadFailureDoesNotSendQueryToProvider(t *testing.T) {
	provider := &searchProvider{}
	service, err := search.New(filepath.Join(t.TempDir(), "search.db"), time.Hour, provider)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Search(context.Background(), "private query"); err == nil {
		t.Fatal("expected closed cache error")
	}
	if provider.calls != 0 {
		t.Fatalf("cache failure leaked query to provider %d time(s)", provider.calls)
	}
}

func TestServiceRejectsOversizedQueryBeforeProvider(t *testing.T) {
	provider := &searchProvider{}
	service, err := search.New(filepath.Join(t.TempDir(), "search.db"), time.Hour, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })

	if _, err := service.Search(context.Background(), strings.Repeat("x", 4097)); err == nil {
		t.Fatal("oversized search query was accepted")
	}
	if provider.calls != 0 {
		t.Fatalf("oversized query reached provider %d time(s)", provider.calls)
	}
}

func TestSearXNGRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"results":[{"title":"` + strings.Repeat("x", 5<<20) + `"}]}`))
	}))
	t.Cleanup(server.Close)

	provider := search.NewSearXNG(server.URL, server.Client())
	if _, err := provider.Search(context.Background(), "bounded response"); err == nil {
		t.Fatal("oversized SearXNG response was accepted")
	}
}
