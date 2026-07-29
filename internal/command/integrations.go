package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/knowledge"
	labmcp "github.com/master-bogdan/local-ai-lab/internal/mcp"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
	"github.com/master-bogdan/local-ai-lab/internal/search"
)

func (r Runner) runMCP(ctx context.Context) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	if !installation.Services.Search && !installation.Services.Knowledge {
		return errors.New("local MCP has no enabled search or knowledge service")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	var web labmcp.WebSearcher
	if installation.Services.Search {
		cacheDir := filepath.Join(installation.DataDir, "cache")
		if err := os.MkdirAll(cacheDir, 0o700); err != nil {
			return err
		}
		searchService, err := search.New(
			filepath.Join(cacheDir, "web-search.db"), 6*time.Hour,
			search.NewSearXNG("http://127.0.0.1:8088", client),
		)
		if err != nil {
			return err
		}
		defer searchService.Close()
		web = searchService
	}
	var knowledgeSearch labmcp.KnowledgeSearcher
	if installation.Services.Knowledge {
		knowledgeSearch = knowledge.NewStore(
			"http://127.0.0.1:11434", "http://127.0.0.1:6333",
			embeddingModel(installation), client,
		)
	}
	return labmcp.Run(ctx, web, knowledgeSearch)
}

func (r Runner) indexWorkspace(ctx context.Context, workspace string) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	if !installation.Services.Knowledge {
		return errors.New("workspace knowledge is not enabled; reinstall from the main menu and select Qdrant")
	}
	absolutePath, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	if err := r.executor.Execute(ctx, labruntime.KnowledgeStartPlan(installation)); err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	store := knowledge.NewStore(
		"http://127.0.0.1:11434", "http://127.0.0.1:6333",
		embeddingModel(installation), client,
	)
	err = r.terminal.RunTask(ctx, "Index workspace", "Workspace indexed", func(taskContext context.Context, output io.Writer) error {
		fmt.Fprintf(output, "workspace  %s\n", absolutePath)
		stats, indexErr := knowledge.NewIndexer(store).Index(taskContext, absolutePath)
		if indexErr != nil {
			return indexErr
		}
		fmt.Fprintf(output, "files      %d\nchunks     %d\n", stats.Files, stats.Chunks)
		fmt.Fprintln(output, "Ollama and Qdrant remain running after indexing.")
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func embeddingModel(installation config.Installation) string {
	if installation.EmbeddingModel != "" {
		return installation.EmbeddingModel
	}
	return "qwen3-embedding:0.6b"
}
