package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/knowledge"
	labmcp "github.com/master-bogdan/local-ai-lab/internal/mcp"
	"github.com/master-bogdan/local-ai-lab/internal/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type webSearcher struct{}

func (webSearcher) Search(context.Context, string) ([]search.Result, error) {
	return []search.Result{{Title: "Go", URL: "https://go.dev", Snippet: "Documentation"}}, nil
}

type knowledgeSearcher struct{}

func (knowledgeSearcher) Search(context.Context, string, int) ([]knowledge.Match, error) {
	return nil, nil
}

type countingWebSearcher struct {
	calls int
}

func (s *countingWebSearcher) Search(context.Context, string) ([]search.Result, error) {
	s.calls++
	return nil, nil
}

func TestServerExposesLocalWebSearchTool(t *testing.T) {
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := labmcp.NewServer(webSearcher{}, knowledgeSearcher{})
	session, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer session.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "web_search", Arguments: map[string]any{"query": "Go documentation"},
	})
	if err != nil {
		t.Fatalf("call web_search: %v", err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "https://go.dev") {
		t.Fatalf("unexpected tool response: %s", text)
	}
}

func TestServerRegistersOnlyAvailableServices(t *testing.T) {
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := labmcp.NewServer(webSearcher{}, nil)
	session, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "web_search" {
		t.Fatalf("unexpected tools for web-only installation: %#v", result.Tools)
	}
}

func TestServerRejectsOversizedQueryBeforeSearch(t *testing.T) {
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	searcher := &countingWebSearcher{}
	server := labmcp.NewServer(searcher, nil)
	session, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "web_search", Arguments: map[string]any{"query": strings.Repeat("x", 4097)},
	})
	if err != nil {
		t.Fatalf("call web_search: %v", err)
	}
	if !result.IsError {
		t.Fatal("oversized MCP query was not reported as a tool error")
	}
	if searcher.calls != 0 {
		t.Fatalf("oversized MCP query reached search %d time(s)", searcher.calls)
	}
}
