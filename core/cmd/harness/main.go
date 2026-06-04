// Command harness is the standalone TUI coding harness — same Go core as the
// VS Code sidecar, no VS Code required.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/patmeh1/foundry-copilot/core/internal/config"
	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
	"github.com/patmeh1/foundry-copilot/core/internal/rag"
	"github.com/patmeh1/foundry-copilot/core/internal/tui"
)

var Version = "0.1.0-dev"

func main() {
	root := &cobra.Command{
		Use:     "foundry-copilot",
		Short:   "Foundry-locked coding harness (sister binary to the VS Code extension).",
		Version: Version,
	}
	root.AddCommand(versionCmd(), initCmd(), chatCmd(), agentCmd(), indexCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version.",
		Run: func(*cobra.Command, []string) {
			fmt.Println(Version)
		},
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialise ~/.foundry-copilot with a default config.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Printf("config loaded; index dir = %s\n", c.RAGIndexDir)
			fmt.Printf("endpoint = %q (set via FOUNDRY_COPILOT_ENDPOINT or the VS Code extension)\n", c.Endpoint)
			return nil
		},
	}
}

func chatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive Foundry-backed chat (TUI).",
		RunE: func(*cobra.Command, []string) error {
			if _, err := config.Load(); err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return tui.Run(ctx, tui.ModeChat)
		},
	}
}

func agentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agent",
		Short: "Start the agent TUI (chat + tool use).",
		RunE: func(*cobra.Command, []string) error {
			if _, err := config.Load(); err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return tui.Run(ctx, tui.ModeAgent)
		},
	}
}

func indexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index [path]",
		Short: "Build the RAG index for a workspace.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.EmbeddingDeployment == "" {
				return fmt.Errorf("embedding_deployment not configured")
			}
			root := cfg.WorkspaceRoot
			if len(args) == 1 {
				root = args[0]
			}
			if root == "" {
				root, _ = os.Getwd()
			}
			abs, err := filepath.Abs(root)
			if err != nil {
				return err
			}
			cred, err := foundry.NewCredential()
			if err != nil {
				return err
			}
			client, err := foundry.NewClient(cfg.Endpoint, cred)
			if err != nil {
				return err
			}
			store, err := rag.Open(filepath.Join(cfg.RAGIndexDir, "store.gob"))
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			files, err := rag.WalkWorkspace(ctx, abs, 4000)
			if err != nil {
				return err
			}
			var chunks []rag.Chunk
			for _, rel := range files {
				cs, err := rag.ChunkFile(filepath.Join(abs, rel), rel, rag.ChunkOptions{})
				if err != nil {
					continue
				}
				chunks = append(chunks, cs...)
			}
			const batch = 64
			for start := 0; start < len(chunks); start += batch {
				end := start + batch
				if end > len(chunks) {
					end = len(chunks)
				}
				inputs := make([]string, end-start)
				for i := start; i < end; i++ {
					inputs[i-start] = chunks[i].Text
				}
				vecs, err := client.Embed(ctx, cfg.EmbeddingDeployment, inputs)
				if err != nil {
					return err
				}
				for i := range vecs {
					chunks[start+i].Vec = vecs[i]
				}
				fmt.Printf("\rembedded %d/%d chunks", end, len(chunks))
			}
			fmt.Println()
			if err := store.Replace(chunks); err != nil {
				return err
			}
			fmt.Printf("indexed %d files → %d chunks (stored in %s)\n",
				len(files), len(chunks), cfg.RAGIndexDir)
			return nil
		},
	}
	return cmd
}
