package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
	"github.com/patmeh1/foundry-copilot/core/internal/rag"
)

// RAGSearch is the agent-facing tool that embeds a query and returns
// the top-K matching chunks from the in-process vector store. It is
// only registered when an index has been built.
type RAGSearch struct {
	Foundry    *foundry.Client
	Store      *rag.Store
	Deployment string
}

func (RAGSearch) Name() string { return "rag_search" }
func (RAGSearch) Description() string {
	return "Semantic search over the indexed workspace. Returns the top matching code/text chunks with file paths and line ranges. Use this to find relevant code before reading files."
}
func (RAGSearch) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Natural-language description of what to find.",
			},
			"k": map[string]any{
				"type":        "integer",
				"description": "Number of results (default 6, max 20).",
			},
		},
		"required": []string{"query"},
	}
}

type ragSearchArgs struct {
	Query string `json:"query"`
	K     int    `json:"k,omitempty"`
}

func (t RAGSearch) Invoke(ctx context.Context, argsJSON string) (string, error) {
	if t.Foundry == nil || t.Store == nil || t.Deployment == "" {
		return "", fmt.Errorf("rag_search: index not available")
	}
	var a ragSearchArgs
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("rag_search: bad args: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return "", fmt.Errorf("rag_search: query required")
	}
	k := a.K
	if k <= 0 {
		k = 6
	}
	if k > 20 {
		k = 20
	}
	vecs, err := t.Foundry.Embed(ctx, t.Deployment, []string{a.Query})
	if err != nil || len(vecs) == 0 {
		return "", fmt.Errorf("rag_search: embed failed: %v", err)
	}
	hits := t.Store.Search(vecs[0], k)
	if len(hits) == 0 {
		return "no results", nil
	}
	var b strings.Builder
	for i, h := range hits {
		fmt.Fprintf(&b, "## %d. %s:L%d-L%d  (score %.3f)\n", i+1, h.Chunk.Path, h.Chunk.StartLine, h.Chunk.EndLine, h.Score)
		b.WriteString("```\n")
		b.WriteString(h.Chunk.Text)
		if !strings.HasSuffix(h.Chunk.Text, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString("```\n\n")
	}
	return b.String(), nil
}
