package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	agentpkg "github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/config"
	"github.com/soasurs/koda/internal/tui"
)

func main() {
	cfg := config.Load()

	// CLI flags override env-based defaults
	flag.StringVar(&cfg.Provider, "provider", cfg.Provider, "LLM provider: openai | anthropic | gemini")
	flag.StringVar(&cfg.Model, "model", cfg.Model, "Model name (e.g. gpt-4o, claude-sonnet-4-5)")
	flag.BoolVar(&cfg.NoSession, "no-session", false, "Disable session persistence (in-memory only)")
	flag.BoolVar(&cfg.SafeMode, "safe", false, "Reserved: require confirmation for shell commands")
	flag.Parse()

	ctx := context.Background()

	rt, err := agentpkg.NewRuntime(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "koda: %v\n", err)
		os.Exit(1)
	}

	// ── one-shot mode ──────────────────────────────────────────────────────
	if flag.NArg() > 0 {
		prompt := flag.Arg(0)
		sessionID, err := rt.NewSession(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "koda: %v\n", err)
			os.Exit(1)
		}
		for event, runErr := range rt.Run(ctx, sessionID, prompt) {
			if runErr != nil {
				fmt.Fprintf(os.Stderr, "koda: %v\n", runErr)
				os.Exit(1)
			}
			if event.Partial {
				fmt.Print(event.Message.Content)
			}
		}
		fmt.Println()
		return
	}

	// ── interactive TUI mode ───────────────────────────────────────────────
	m := tui.New(rt)
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "koda: %v\n", err)
		os.Exit(1)
	}
}
