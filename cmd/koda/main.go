package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

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
	flag.BoolVar(&cfg.SafeMode, "safe", cfg.SafeMode, "Require confirmation before mutating tool calls execute")
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
		runCtx := ctx
		if rt.SafeMode() {
			runCtx = agentpkg.WithToolConfirmation(runCtx, promptToolConfirmation)
		}
		for event, runErr := range rt.Run(runCtx, sessionID, prompt) {
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
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "koda: %v\n", err)
		os.Exit(1)
	}
}

func promptToolConfirmation(ctx context.Context, req agentpkg.ToolConfirmationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "\n[safe-mode] Allow tool call?\ntool: %s\n", req.ToolName)
	if summary := strings.TrimSpace(req.Summary); summary != "" {
		fmt.Fprintf(os.Stderr, "%s\n", summary)
	}
	if args := strings.TrimSpace(req.Arguments); args != "" && args != strings.TrimSpace(req.Summary) {
		fmt.Fprintf(os.Stderr, "args: %s\n", args)
	}
	fmt.Fprint(os.Stderr, "Approve? [y/N]: ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read shell confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "y" || answer == "yes" {
		return nil
	}
	return fmt.Errorf("command rejected by user")
}
