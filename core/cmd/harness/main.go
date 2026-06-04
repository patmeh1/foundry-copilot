// Command harness is the standalone TUI coding harness — same Go core as the
// VS Code sidecar, no VS Code required. Phase 9 wires the Bubble Tea UI.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/patmeh1/foundry-copilot/core/internal/config"
)

var Version = "0.1.0-dev"

func main() {
	root := &cobra.Command{
		Use:     "foundry-copilot",
		Short:   "Foundry-locked coding harness (sister binary to the VS Code extension).",
		Version: Version,
	}
	root.AddCommand(versionCmd(), initCmd(), chatCmd(), indexCmd())
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
			return nil
		},
	}
}

func chatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive Foundry-backed chat (TUI). [Phase 9]",
		RunE: func(*cobra.Command, []string) error {
			fmt.Println("Bubble Tea TUI lands in Phase 9.")
			return nil
		},
	}
}

func indexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index [path]",
		Short: "Build the RAG index for a workspace. [Phase 7]",
		RunE: func(*cobra.Command, []string) error {
			fmt.Println("RAG indexing lands in Phase 7.")
			return nil
		},
	}
}
