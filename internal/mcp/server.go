package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/master-bogdan/local-ai-lab/internal/knowledge"
	"github.com/master-bogdan/local-ai-lab/internal/search"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxQueryBytes = 4096

type WebSearcher interface {
	Search(context.Context, string) ([]search.Result, error)
}

type KnowledgeSearcher interface {
	Search(context.Context, string, int) ([]knowledge.Match, error)
}

type queryArgs struct {
	Query string `json:"query" jsonschema:"search query"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum results"`
}

func NewServer(web WebSearcher, knowledgeSearch KnowledgeSearcher) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "local-ai-lab", Version: "1.0.0"}, nil)
	if web != nil {
		sdk.AddTool(server, &sdk.Tool{
			Name: "web_search", Description: "Search the public web through local SearXNG with SQLite caching",
		}, func(ctx context.Context, _ *sdk.CallToolRequest, args queryArgs) (*sdk.CallToolResult, any, error) {
			query, err := validateQuery(args.Query)
			if err != nil {
				return nil, nil, err
			}
			results, err := web.Search(ctx, query)
			return toolResult(results, err)
		})
	}
	if knowledgeSearch != nil {
		sdk.AddTool(server, &sdk.Tool{
			Name: "knowledge_search", Description: "Search locally indexed workspace knowledge in Qdrant",
		}, func(ctx context.Context, _ *sdk.CallToolRequest, args queryArgs) (*sdk.CallToolResult, any, error) {
			limit := args.Limit
			if limit < 1 || limit > 20 {
				limit = 5
			}
			query, err := validateQuery(args.Query)
			if err != nil {
				return nil, nil, err
			}
			results, err := knowledgeSearch.Search(ctx, query, limit)
			return toolResult(results, err)
		})
	}
	return server
}

func validateQuery(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", errors.New("search query is required")
	}
	if len(query) > maxQueryBytes {
		return "", fmt.Errorf("search query exceeds %d bytes", maxQueryBytes)
	}
	return query, nil
}

func Run(ctx context.Context, web WebSearcher, knowledgeSearch KnowledgeSearcher) error {
	return NewServer(web, knowledgeSearch).Run(ctx, &sdk.StdioTransport{})
}

func toolResult(value any, err error) (*sdk.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("encode tool result: %w", err)
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(payload)}}}, value, nil
}
