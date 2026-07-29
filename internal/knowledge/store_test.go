package knowledge_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/knowledge"
)

func TestStoreRejectsOversizedEmbeddingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"embeddings":[[` + strings.Repeat("0,", 5<<20) + `1]]}`))
	}))
	t.Cleanup(server.Close)

	store := knowledge.NewStore(server.URL, server.URL, "embedding-model", server.Client())
	if _, err := store.Embed(context.Background(), "bounded response"); err == nil {
		t.Fatal("oversized embedding response was accepted")
	}
}
