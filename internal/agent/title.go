package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/soasurs/adk/model"

	"github.com/soasurs/koda/internal/store"
)

const titleMaxRunes = 80

// GenerateTitle uses the session's configured provider and model to derive a
// concise title from the first user input. It does not use tools or session
// history and does not persist the generated title.
func (f *Factory) GenerateTitle(ctx context.Context, session store.Session, input model.Content) (string, error) {
	if err := validateSession(session); err != nil {
		return "", err
	}
	if input.Role != model.RoleUser {
		return "", errors.New("agent: title input must have user role")
	}
	value, _, err := f.resolveProviderAndModel(ctx, session)
	if err != nil {
		return "", err
	}
	llm, err := f.newModel(ctx, value, session.ModelID, "")
	if err != nil {
		return "", err
	}
	prompt, err := embeddedPrompt("prompts/title.md")
	if err != nil {
		return "", err
	}
	request := &model.LLMRequest{
		Model: session.ModelID,
		Contents: []model.Content{
			{Role: model.RoleSystem, Content: prompt},
			input,
		},
	}
	var title string
	for response, err := range llm.GenerateContent(ctx, request, &model.GenerateConfig{MaxTokens: 64}, false) {
		if err != nil {
			return "", fmt.Errorf("agent: generate session title: %w", err)
		}
		if response == nil {
			return "", errors.New("agent: generate session title: model returned nil response")
		}
		if !response.Partial && response.Content.Role == model.RoleAssistant {
			title = response.Content.Content
		}
	}
	return normalizeGeneratedTitle(title)
}

func normalizeGeneratedTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if line, _, found := strings.Cut(title, "\n"); found {
		title = strings.TrimSpace(line)
	}
	title = strings.Trim(title, "`\"'“”‘’")
	title = strings.TrimSpace(title)
	if title == "" {
		return "", errors.New("agent: generated session title is empty")
	}
	if utf8.RuneCountInString(title) > titleMaxRunes {
		title = string([]rune(title)[:titleMaxRunes])
	}
	return title, nil
}
