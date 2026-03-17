package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	textarea "github.com/jbarrieault/bubble-textarea"

	"github.com/soasurs/koda/internal/agent"

	amodel "github.com/soasurs/adk/model"
	"github.com/soasurs/adk/session/message"
)

const (
	statusBarHeight = 1
	commandListGap  = 1
)

type Model struct {
	width  int
	height int

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	msgs        []ChatMessage
	focusMsgIdx int

	runtime       *agent.Runtime
	sessionID     int64
	hasSession    bool
	contextTokens int64
	eventChan     chan runnerMsg

	running bool
	status  string
	err     error

	cancelAgent context.CancelFunc
	escPending  bool

	slash   slashState
	chooser chooserState
}

func New(rt *agent.Runtime) Model {
	ta := textarea.New()
	ta.Placeholder = "Message koda..."
	ta.Prompt = ""
	ta.Focus()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.SetMaxVisualHeight(5)
	ta.MaxWidth = 0
	ta.KeyMap.InsertNewline.SetEnabled(false)
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.CursorLineNumber = lipgloss.NewStyle()
	ta.FocusedStyle.EndOfBuffer = lipgloss.NewStyle()
	ta.BlurredStyle = ta.FocusedStyle

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	vp := viewport.New(80, 20)

	m := Model{
		viewport:    vp,
		textarea:    ta,
		spinner:     sp,
		focusMsgIdx: -1,
		runtime:     rt,
	}
	m.syncSlashState()
	return m
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width)
		vpHeight := msg.Height - m.currentComposerHeight() - statusBarHeight - m.commandListHeight()
		if vpHeight < 1 {
			vpHeight = 1
		}
		m.viewport.Width = msg.Width
		m.viewport.Height = vpHeight
		m.refreshViewport()
		return m, nil

	case tea.KeyMsg:
		if m.chooser.Stage != chooserStageNone {
			return m.handleChooserKey(msg)
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "enter":
			if m.running {
				return m, nil
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}
			if input == "/connect" {
				m.openProviderChooser()
				m.refreshViewport()
				return m, nil
			}
			if input == "/model" {
				if err := m.runtime.RefreshModels(context.Background()); err != nil {
					m.err = nil
					m.msgs = append(m.msgs, ChatMessage{Kind: KindSystem, Content: "Unable to load live model list, using built-in defaults: " + err.Error()})
				}
				m.openModelChooser()
				m.refreshViewport()
				return m, nil
			}
			if input == "/sessions" {
				sessions, err := m.runtime.ListSessions(context.Background())
				if err != nil {
					m.err = err
					m.msgs = append(m.msgs, ChatMessage{Kind: KindSystem, Content: "Unable to load sessions: " + err.Error()})
					m.refreshViewport()
					return m, textarea.Blink
				}
				m.openSessionsChooser(sessions)
				m.refreshViewport()
				return m, nil
			}
			if m.shouldApplySlashSelection(input) {
				m.applySelectedSlashOption()
				m.syncSlashState()
				m.refreshViewport()
				return m, nil
			}
			if strings.HasPrefix(input, "/") {
				cmd := strings.Fields(input)
				if len(cmd) > 0 {
					m.textarea.Reset()
					m.syncSlashState()
					if cmd[0] == "/compact" {
						m.running = true
						m.status = "compacting..."
						return m, tea.Batch(m.runCommand(cmd[0], strings.TrimSpace(strings.TrimPrefix(input, cmd[0]))), m.spinner.Tick)
					}
					return m, m.runCommand(cmd[0], strings.TrimSpace(strings.TrimPrefix(input, cmd[0])))
				}
			}
			isNewSession := !m.hasSession
			if !m.hasSession {
				sessionID, err := m.runtime.NewSession(context.Background())
				if err != nil {
					m.err = err
					m.msgs = append(m.msgs, ChatMessage{Kind: KindSystem, Content: "Failed to start session: " + err.Error()})
					m.refreshViewport()
					m.viewport.GotoBottom()
					return m, textarea.Blink
				}
				m.sessionID = sessionID
				m.hasSession = true
			}
			m.textarea.Reset()
			m.msgs = append(m.msgs, ChatMessage{Kind: KindUser, Content: input})
			if err := m.runtime.TouchSession(context.Background(), m.sessionID, ""); err != nil {
				m.msgs = append(m.msgs, ChatMessage{Kind: KindSystem, Content: "Failed to update session metadata: " + err.Error()})
			}
			m.refreshViewport()
			m.viewport.GotoBottom()
			m.running = true
			if m.runtime.Mode() == agent.ModePlan {
				m.status = "planning..."
			} else {
				m.status = "thinking..."
			}
			m.err = nil
			m.eventChan = make(chan runnerMsg, 32)
			startCmd := m.launchAgent(input)
			cmds = append(cmds, startCmd, m.spinner.Tick)
			if isNewSession {
				cmds = append(cmds, m.launchTitleGen(m.sessionID, input))
			}
			return m, tea.Batch(cmds...)

		case "up", "ctrl+p":
			if m.slash.Visible() {
				m.selectSlash(-1)
				return m, nil
			}

		case "down", "ctrl+n":
			if m.slash.Visible() {
				m.selectSlash(1)
				return m, nil
			}

		case "tab":
			if m.slash.Visible() {
				m.applySelectedSlashOption()
				m.syncSlashState()
				return m, nil
			}
			newMode := agent.ModeBuild
			if m.runtime.Mode() == agent.ModeBuild {
				newMode = agent.ModePlan
			}
			if err := m.runtime.SetMode(context.Background(), newMode); err != nil {
				m.err = err
				return m, nil
			}
			if newMode == agent.ModePlan {
				m.textarea.Placeholder = "Message Architect (Plan)..."
			} else {
				m.textarea.Placeholder = "Message koda (Build)..."
			}
			m.refreshViewport()
			m.viewport.GotoBottom()
			return m, nil

		case "esc":
			if m.running {
				if m.escPending {
					m.escPending = false
					if m.cancelAgent != nil {
						m.cancelAgent()
					}
					m.msgs = append(m.msgs, ChatMessage{Kind: KindSystem, Content: "Agent run cancelled."})
					m.refreshViewport()
					return m, nil
				}
				m.escPending = true
				m.refreshViewport()
				return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return escTimeoutMsg{} })
			}
			if m.slash.Mode != slashModeNone {
				m.textarea.Reset()
				m.syncSlashState()
				m.refreshViewport()
				return m, nil
			}

		case "[":
			m.moveFocus(-1)
			m.refreshViewport()
			return m, nil

		case "]":
			m.moveFocus(+1)
			m.refreshViewport()
			return m, nil

		case "x":
			m.toggleFocused()
			m.refreshViewport()
			return m, nil

		case "pgup":
			m.viewport.HalfViewUp()
			return m, nil

		case "pgdown":
			m.viewport.HalfViewDown()
			return m, nil
		}

		if !m.running {
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			m.syncSlashState()
			m.refreshViewport()
			cmds = append(cmds, taCmd)
		}
		return m, tea.Batch(cmds...)

	case runnerMsg:
		if msg.done {
			m.running = false
			m.escPending = false
			m.cancelAgent = nil
			m.refreshViewport()
			cmds = append(cmds, textarea.Blink)
			return m, tea.Batch(cmds...)
		}
		if msg.err != nil {
			m.running = false
			m.escPending = false
			m.cancelAgent = nil
			if !errors.Is(msg.err, context.Canceled) {
				m.err = msg.err
			}
			m.refreshViewport()
			cmds = append(cmds, textarea.Blink)
			return m, tea.Batch(cmds...)
		}

		m.applyEvent(msg.event)
		m.refreshViewport()
		m.viewport.GotoBottom()
		cmds = append(cmds, waitForRunnerMsg(m.eventChan))
		return m, tea.Batch(cmds...)

	case commandResultMsg:
		m.running = false
		if msg.err != nil {
			_ = m.closeChooser()
			m.err = msg.err
			m.msgs = append(m.msgs, ChatMessage{Kind: KindSystem, Content: "Command failed: " + msg.err.Error()})
		} else if msg.ack != "" {
			m.err = nil
			if msg.sessionID != 0 {
				m.sessionID = msg.sessionID
			}
			if msg.setSession {
				m.hasSession = msg.hasSession
				if !msg.hasSession {
					m.sessionID = 0
				}
			}
			if msg.msgs != nil {
				m.msgs = chatMessagesFromSession(msg.msgs)
				m.contextTokens = 0
				for i := len(msg.msgs) - 1; i >= 0; i-- {
					if msg.msgs[i].PromptTokens > 0 {
						m.contextTokens = msg.msgs[i].PromptTokens
						break
					}
				}
			}
			m.msgs = append(m.msgs, ChatMessage{Kind: KindSystem, Content: msg.ack})
			if msg.models != nil {
				m.openModelChooserWithModels(msg.models)
				m.refreshViewport()
				return m, nil
			}
			if msg.sessions != nil {
				m.openSessionsChooser(msg.sessions)
				m.refreshViewport()
				return m, nil
			}
		}
		m.refreshViewport()
		m.viewport.GotoBottom()
		return m, textarea.Blink

	case titleMsg:
		if msg.err == nil && msg.title != "" {
			_ = m.runtime.SetSessionTitle(context.Background(), msg.sessionID, msg.title)
		}
		return m, nil

	case escTimeoutMsg:
		if m.escPending {
			m.escPending = false
			m.refreshViewport()
		}
		return m, nil

	case spinner.TickMsg:
		if m.running {
			var spCmd tea.Cmd
			m.spinner, spCmd = m.spinner.Update(msg)
			cmds = append(cmds, spCmd)
		}
		return m, tea.Batch(cmds...)
	}

	if msg == nil {
		return m, nil
	}

	var vpCmd, taCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	if !m.running {
		m.textarea, taCmd = m.textarea.Update(msg)
		m.syncSlashState()
		m.refreshViewport()
	}
	return m, tea.Batch(vpCmd, taCmd)
}

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	parts := []string{m.viewport.View(), m.renderStatusBar()}
	if m.slash.Visible() {
		parts = append(parts, m.renderSlashMenu())
	}
	parts = append(parts, m.renderComposer())
	view := lipgloss.JoinVertical(lipgloss.Left, parts...)
	if m.chooser.Stage != chooserStageNone {
		return m.renderChooserModal()
	}
	return view
}

func (m *Model) openProviderChooser() {
	providers := m.runtime.AvailableProviders()
	choices := make([]providerChoice, 0, len(providers))
	selected := 0
	for i, provider := range providers {
		choices = append(choices, providerChoice{Label: provider.Label, Value: provider.Value})
		if provider.Value == m.runtime.Provider() {
			selected = i
		}
	}
	m.chooser = chooserState{
		Stage:            chooserStageProvider,
		Title:            "Connect Provider",
		Prompt:           "Choose a provider with up/down, then press Enter.",
		Hint:             "Esc closes this dialog.",
		Providers:        choices,
		SelectedProvider: selected,
	}
	m.textarea.Blur()
}

func (m *Model) openModelChooser() {
	m.openModelChooserWithModels(m.runtime.AvailableModels())
	if m.chooser.Stage != chooserStageNone {
		m.textarea.Blur()
	}
}

func (m *Model) openModelChooserWithModels(models []string) {
	if len(models) == 0 {
		m.msgs = append(m.msgs, ChatMessage{Kind: KindSystem, Content: "No models are available for the current provider."})
		return
	}
	selected := 0
	for i, model := range models {
		if model == m.runtime.ModelName() {
			selected = i
			break
		}
	}
	m.chooser = chooserState{
		Stage:         chooserStageModel,
		Title:         "Choose Model",
		Prompt:        "Select a model.",
		Hint:          "Esc closes this dialog.",
		Models:        models,
		SelectedModel: selected,
		ModelOffset:   max(0, selected-5),
		Query:         "",
	}
	m.ensureModelSelectionVisible()
}

func (m *Model) openSessionsChooser(sessions []agent.SessionMeta) {
	if len(sessions) == 0 {
		m.msgs = append(m.msgs, ChatMessage{Kind: KindSystem, Content: "No saved sessions yet."})
		m.chooser = chooserState{} // Ensure chooser is cleared
		return
	}
	selected := 0
	for i, sess := range sessions {
		if sess.SessionID == m.sessionID {
			selected = i
			break
		}
	}
	if selected >= len(sessions) {
		selected = 0
	}
	m.chooser = chooserState{
		Stage:           chooserStageSessions,
		Title:           "Choose Session",
		Prompt:          "Switch to a previous session.",
		Hint:            "d: Delete, Esc: Close.",
		Sessions:        sessions,
		SelectedSession: selected,
		SessionOffset:   max(0, selected-5),
	}
	m.ensureSessionSelectionVisible()
	m.textarea.Blur()
}

func (m *Model) closeChooser() tea.Cmd {
	m.chooser = chooserState{}
	m.textarea.SetValue("")
	m.syncSlashState()
	return m.textarea.Focus()
}

func (m Model) handleChooserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.chooser.Stage {
	case chooserStageProvider:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			cmd := m.closeChooser()
			m.refreshViewport()
			return m, cmd
		case "up", "ctrl+p":
			if len(m.chooser.Providers) > 0 {
				m.chooser.SelectedProvider--
				if m.chooser.SelectedProvider < 0 {
					m.chooser.SelectedProvider = len(m.chooser.Providers) - 1
				}
			}
			return m, nil
		case "down", "ctrl+n":
			if len(m.chooser.Providers) > 0 {
				m.chooser.SelectedProvider++
				if m.chooser.SelectedProvider >= len(m.chooser.Providers) {
					m.chooser.SelectedProvider = 0
				}
			}
			return m, nil
		case "enter":
			if len(m.chooser.Providers) == 0 {
				return m, nil
			}
			m.chooser.Provider = m.chooser.Providers[m.chooser.SelectedProvider]
			m.chooser.Stage = chooserStageAPIKey
			m.chooser.APIKeyInput = ""
			m.chooser.Title = "Provider API Key"
			m.chooser.Prompt = "Enter the API key for " + m.chooser.Provider.Label + "."
			m.chooser.Hint = "Enter saves, Esc goes back."
			m.refreshViewport()
			return m, nil
		}

	case chooserStageAPIKey:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.chooser.Stage = chooserStageProvider
			m.chooser.Title = "Connect Provider"
			m.chooser.Prompt = "Choose a provider with up/down, then press Enter."
			m.chooser.Hint = "Esc closes this dialog."
			m.chooser.APIKeyInput = ""
			m.refreshViewport()
			return m, nil
		case "enter":
			provider := m.chooser.Provider
			apiKey := strings.TrimSpace(m.chooser.APIKeyInput)
			cmd := m.closeChooser()
			m.refreshViewport()
			return m, tea.Batch(cmd, m.runConnectSelection(provider, apiKey))
		case "backspace", "ctrl+h":
			runes := []rune(m.chooser.APIKeyInput)
			if len(runes) > 0 {
				m.chooser.APIKeyInput = string(runes[:len(runes)-1])
			}
			return m, nil
		}

		if len(msg.Runes) > 0 {
			m.chooser.APIKeyInput += string(msg.Runes)
			return m, nil
		}

	case chooserStageModel:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			cmd := m.closeChooser()
			m.refreshViewport()
			return m, cmd
		case "backspace", "ctrl+h":
			runes := []rune(m.chooser.Query)
			if len(runes) > 0 {
				m.chooser.Query = string(runes[:len(runes)-1])
				m.resetModelChooserSelection()
			}
			return m, nil
		case "up", "ctrl+p":
			filtered := m.filteredChooserModels()
			if len(filtered) > 0 {
				m.chooser.SelectedModel--
				if m.chooser.SelectedModel < 0 {
					m.chooser.SelectedModel = len(filtered) - 1
				}
				m.ensureModelSelectionVisible()
			}
			return m, nil
		case "down", "ctrl+n":
			filtered := m.filteredChooserModels()
			if len(filtered) > 0 {
				m.chooser.SelectedModel++
				if m.chooser.SelectedModel >= len(filtered) {
					m.chooser.SelectedModel = 0
				}
				m.ensureModelSelectionVisible()
			}
			return m, nil
		case "enter":
			filtered := m.filteredChooserModels()
			if len(filtered) == 0 {
				return m, nil
			}
			selected := filtered[m.chooser.SelectedModel]
			cmd := m.closeChooser()
			m.refreshViewport()
			return m, tea.Batch(cmd, m.runModelSelection(selected))
		}

		if len(msg.Runes) > 0 {
			m.chooser.Query += string(msg.Runes)
			m.resetModelChooserSelection()
			return m, nil
		}

	case chooserStageSessions:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			cmd := m.closeChooser()
			m.refreshViewport()
			return m, cmd
		case "up", "ctrl+p":
			if len(m.chooser.Sessions) > 0 {
				m.chooser.SelectedSession--
				if m.chooser.SelectedSession < 0 {
					m.chooser.SelectedSession = len(m.chooser.Sessions) - 1
				}
				m.ensureSessionSelectionVisible()
			}
			return m, nil
		case "down", "ctrl+n":
			if len(m.chooser.Sessions) > 0 {
				m.chooser.SelectedSession++
				if m.chooser.SelectedSession >= len(m.chooser.Sessions) {
					m.chooser.SelectedSession = 0
				}
				m.ensureSessionSelectionVisible()
			}
			return m, nil
		case "d":
			if len(m.chooser.Sessions) == 0 {
				return m, nil
			}
			selected := m.chooser.Sessions[m.chooser.SelectedSession]
			return m, m.runSessionDelete(selected)

		case "enter":
			if len(m.chooser.Sessions) == 0 {
				return m, nil
			}
			selected := m.chooser.Sessions[m.chooser.SelectedSession]
			cmd := m.closeChooser()
			m.refreshViewport()
			return m, tea.Batch(cmd, m.runSessionSelection(selected))
		}
	}

	return m, nil
}

func (m *Model) runModelSelection(modelName string) tea.Cmd {
	ctx := context.Background()
	hasSession := m.hasSession
	sessionID := m.sessionID
	return func() tea.Msg {
		if err := m.runtime.SetModel(ctx, modelName); err != nil {
			return commandResultMsg{cmd: "/model", err: err}
		}
		result := commandResultMsg{cmd: "/model", ack: fmt.Sprintf("Model set to `%s`.", m.runtime.ModelName()), setSession: true, hasSession: hasSession}
		if hasSession {
			msgs, err := m.runtime.SessionMessages(ctx, sessionID)
			if err != nil {
				return commandResultMsg{cmd: "/model", err: err}
			}
			result.msgs = msgs
		}
		return result
	}
}

func (m *Model) runConnectSelection(provider providerChoice, apiKey string) tea.Cmd {
	ctx := context.Background()
	hasSession := m.hasSession
	sessionID := m.sessionID
	return func() tea.Msg {
		if err := m.runtime.SetProviderAPIKey(ctx, provider.Value, apiKey); err != nil {
			return commandResultMsg{cmd: "/connect", err: err}
		}
		if err := m.runtime.RefreshModels(ctx); err != nil {
			return commandResultMsg{cmd: "/connect", err: err}
		}
		keyMsg := "updated"
		if strings.TrimSpace(apiKey) == "" {
			keyMsg = "kept from environment/default config"
		}
		result := commandResultMsg{
			cmd:        "/connect",
			ack:        fmt.Sprintf("Provider set to %s. API key %s. Choose a model next.", provider.Label, keyMsg),
			setSession: true,
			hasSession: hasSession,
			models:     m.runtime.AvailableModels(),
		}
		if hasSession {
			msgs, err := m.runtime.SessionMessages(ctx, sessionID)
			if err != nil {
				return commandResultMsg{cmd: "/connect", err: err}
			}
			result.msgs = msgs
		}
		return result
	}
}

func (m *Model) runSessionSelection(selected agent.SessionMeta) tea.Cmd {
	ctx := context.Background()
	return func() tea.Msg {
		msgs, err := m.runtime.SessionMessages(ctx, selected.SessionID)
		if err != nil {
			return commandResultMsg{cmd: "/sessions", err: err}
		}
		return commandResultMsg{
			cmd:        "/sessions",
			ack:        fmt.Sprintf("Switched to session %s.", formatSessionLabel(selected)),
			sessionID:  selected.SessionID,
			setSession: true,
			hasSession: true,
			msgs:       msgs,
		}
	}
}

func (m *Model) runSessionDelete(selected agent.SessionMeta) tea.Cmd {
	ctx := context.Background()
	return func() tea.Msg {
		if err := m.runtime.DeleteSession(ctx, selected.SessionID); err != nil {
			return commandResultMsg{cmd: "/sessions", err: err}
		}
		sessions, err := m.runtime.ListSessions(ctx)
		if err != nil {
			return commandResultMsg{cmd: "/sessions", err: err}
		}
		return commandResultMsg{
			cmd:      "/sessions",
			ack:      fmt.Sprintf("Deleted session %s.", formatSessionLabel(selected)),
			sessions: sessions,
		}
	}
}

func (m Model) renderChooserModal() string {
	boxWidth := minInt(56, max(36, m.width-10))
	box := modalBoxStyle.Width(boxWidth).Render(modalWindowStyle.Width(boxWidth - 6).Render(m.renderChooserModalBody(boxWidth - 6)))
	return modalBackdropStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box),
	)
}

func (m Model) renderChooserModalBody(width int) string {
	var lines []string
	fitWidth := func(style lipgloss.Style) int {
		inner := width - style.GetHorizontalFrameSize()
		if inner < 0 {
			return 0
		}
		return inner
	}
	lines = append(lines, modalTitleStyle.Render(m.chooser.Title), "")

	switch m.chooser.Stage {
	case chooserStageProvider:
		lines = append(lines, modalBodyStyle.Width(width).Render(m.chooser.Prompt), "")
		for i, provider := range m.chooser.Providers {
			style := modalOptionStyle
			prefix := "  "
			if i == m.chooser.SelectedProvider {
				style = modalOptionSelectedStyle
				prefix = "> "
			}
			lines = append(lines, style.Width(fitWidth(style)).Render(prefix+provider.Label))
		}
		lines = append(lines, "", modalHintStyle.Render(m.chooser.Hint))

	case chooserStageAPIKey:
		masked := strings.Repeat("*", len([]rune(m.chooser.APIKeyInput)))
		if masked == "" {
			masked = "(leave empty to use env/default key)"
		}
		lines = append(lines, modalBodyStyle.Width(width).Render(m.chooser.Prompt), "")
		lines = append(lines, modalOptionSelectedStyle.Width(fitWidth(modalOptionSelectedStyle)).Render(masked), "")
		lines = append(lines, modalHintStyle.Render(m.chooser.Hint))

	case chooserStageModel:
		lines = append(lines, modalBodyStyle.Width(width).Render(m.chooser.Prompt))
		searchValue := m.chooser.Query
		if searchValue == "" {
			searchValue = "type to filter"
		}
		lines = append(lines, modalInputStyle.Width(fitWidth(modalInputStyle)).Render("Search: "+searchValue), "")
		filtered := m.filteredChooserModels()
		visibleStart, visibleEnd := m.modelWindowRange(len(filtered), m.getModelWindowSize())
		visible := filtered[visibleStart:visibleEnd]
		if len(filtered) == 0 {
			lines = append(lines, modalHintStyle.Render("No matching models"))
			lines = append(lines, modalHintStyle.Render(m.chooser.Hint))
			break
		}
		for i, model := range visible {
			absoluteIdx := visibleStart + i
			style := modalOptionStyle
			prefix := "  "
			if absoluteIdx == m.chooser.SelectedModel {
				style = modalOptionSelectedStyle
				prefix = "> "
			}
			lines = append(lines, style.Width(fitWidth(style)).Render(prefix+model))
		}
		lines = append(lines, "", modalHintStyle.Render(fmt.Sprintf("%d models", len(filtered))))
		lines = append(lines, modalHintStyle.Render(m.chooser.Hint))

	case chooserStageSessions:
		lines = append(lines, modalBodyStyle.Width(width).Render(m.chooser.Prompt), "")
		visibleStart, visibleEnd := m.sessionWindowRange(len(m.chooser.Sessions), m.getSessionWindowSize())
		visible := m.chooser.Sessions[visibleStart:visibleEnd]
		for i, sess := range visible {
			absoluteIdx := visibleStart + i
			style := modalOptionStyle
			prefix := "  "
			if absoluteIdx == m.chooser.SelectedSession {
				style = modalOptionSelectedStyle
				prefix = "> "
			}
			lines = append(lines, style.Width(fitWidth(style)).Render(prefix+formatSessionLabel(sess)))
			lines = append(lines, modalHintStyle.Width(width).Render("   "+formatSessionMeta(sess)))
		}
		lines = append(lines, "", modalHintStyle.Render(fmt.Sprintf("%d sessions", len(m.chooser.Sessions))))
		lines = append(lines, modalHintStyle.Render(m.chooser.Hint))
	}

	return strings.Join(lines, "\n")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) getModelWindowSize() int {
	return maxInt(3, m.height-14)
}

func (m Model) getSessionWindowSize() int {
	return maxInt(2, (m.height-12)/2)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func (m *Model) ensureModelSelectionVisible() {
	windowSize := m.getModelWindowSize()
	filteredCount := len(m.filteredChooserModels())
	if filteredCount <= windowSize {
		m.chooser.ModelOffset = 0
		return
	}
	if m.chooser.SelectedModel < m.chooser.ModelOffset {
		m.chooser.ModelOffset = m.chooser.SelectedModel
	}
	if m.chooser.SelectedModel >= m.chooser.ModelOffset+windowSize {
		m.chooser.ModelOffset = m.chooser.SelectedModel - windowSize + 1
	}
	maxOffset := filteredCount - windowSize
	if m.chooser.ModelOffset > maxOffset {
		m.chooser.ModelOffset = maxOffset
	}
	if m.chooser.ModelOffset < 0 {
		m.chooser.ModelOffset = 0
	}
}

func (m Model) modelWindowRange(total, windowSize int) (int, int) {
	if total <= windowSize {
		return 0, total
	}
	start := m.chooser.ModelOffset
	if start < 0 {
		start = 0
	}
	end := start + windowSize
	if end > total {
		end = total
		start = end - windowSize
	}
	if start < 0 {
		start = 0
	}
	return start, end
}

func (m *Model) resetModelChooserSelection() {
	m.chooser.SelectedModel = 0
	m.chooser.ModelOffset = 0
	m.ensureModelSelectionVisible()
}

func (m *Model) ensureSessionSelectionVisible() {
	windowSize := m.getSessionWindowSize()
	count := len(m.chooser.Sessions)
	if count <= windowSize {
		m.chooser.SessionOffset = 0
		return
	}
	if m.chooser.SelectedSession < m.chooser.SessionOffset {
		m.chooser.SessionOffset = m.chooser.SelectedSession
	}
	if m.chooser.SelectedSession >= m.chooser.SessionOffset+windowSize {
		m.chooser.SessionOffset = m.chooser.SelectedSession - windowSize + 1
	}
	maxOffset := count - windowSize
	if m.chooser.SessionOffset > maxOffset {
		m.chooser.SessionOffset = maxOffset
	}
	if m.chooser.SessionOffset < 0 {
		m.chooser.SessionOffset = 0
	}
}

func (m Model) sessionWindowRange(total, windowSize int) (int, int) {
	if total <= windowSize {
		return 0, total
	}
	start := m.chooser.SessionOffset
	if start < 0 {
		start = 0
	}
	end := start + windowSize
	if end > total {
		end = total
		start = end - windowSize
	}
	if start < 0 {
		start = 0
	}
	return start, end
}

func (m Model) filteredChooserModels() []string {
	query := strings.ToLower(strings.TrimSpace(m.chooser.Query))
	if query == "" {
		return append([]string(nil), m.chooser.Models...)
	}
	filtered := make([]string, 0, len(m.chooser.Models))
	for _, model := range m.chooser.Models {
		if strings.Contains(strings.ToLower(model), query) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func (m *Model) launchAgent(input string) tea.Cmd {
	ch := m.eventChan
	rt := m.runtime
	sessionID := m.sessionID
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelAgent = cancel

	go func() {
		defer func() {
			cancel()
			ch <- runnerMsg{done: true}
		}()
		for event, err := range rt.Run(ctx, sessionID, input) {
			ch <- runnerMsg{event: event, err: err}
			if err != nil {
				return
			}
		}
	}()
	return waitForRunnerMsg(ch)
}

func (m *Model) launchTitleGen(sessionID int64, userInput string) tea.Cmd {
	rt := m.runtime
	return func() tea.Msg {
		title, err := rt.GenerateTitle(context.Background(), userInput)
		return titleMsg{sessionID: sessionID, title: title, err: err}
	}
}

func waitForRunnerMsg(ch chan runnerMsg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func (m *Model) runCommand(cmd, arg string) tea.Cmd {
	ctx := context.Background()
	cmd = strings.TrimSpace(cmd)
	arg = strings.TrimSpace(arg)

	return func() tea.Msg {
		switch cmd {
		case "/new":
			return commandResultMsg{cmd: cmd, ack: "Started a new empty session.", setSession: true, hasSession: false, msgs: []*message.Message{}}

		case "/connect":
			return commandResultMsg{cmd: cmd, ack: "Press Enter on `/connect` to open the provider popup."}

		case "/model":
			return commandResultMsg{cmd: cmd, ack: "Press Enter on `/model` to open the model popup."}

		case "/sessions":
			sessions, err := m.runtime.ListSessions(ctx)
			if err != nil {
				return commandResultMsg{cmd: cmd, err: err}
			}
			return commandResultMsg{cmd: cmd, ack: "Choose a saved session.", sessions: sessions}

		case "/compact":
			if !m.hasSession {
				return commandResultMsg{cmd: cmd, ack: "Nothing to compact yet; this session has not started."}
			}
			result, err := m.runtime.CompactSession(ctx, m.sessionID)
			if err != nil {
				return commandResultMsg{cmd: cmd, err: err}
			}
			msgs, err := m.runtime.SessionMessages(ctx, m.sessionID)
			if err != nil {
				return commandResultMsg{cmd: cmd, err: err}
			}
			if result.ArchivedMessages == 0 {
				return commandResultMsg{cmd: cmd, ack: "Nothing to compact yet; keeping the current session as-is.", setSession: true, hasSession: true, msgs: msgs}
			}
			return commandResultMsg{cmd: cmd, ack: fmt.Sprintf("Compacted session: %d -> %d messages, archived %d, kept %d recent messages plus a summary.", result.BeforeMessages, result.AfterMessages, result.ArchivedMessages, result.KeptMessages), setSession: true, hasSession: true, msgs: msgs}

		default:
			return commandResultMsg{cmd: cmd, err: fmt.Errorf("unknown command %q", cmd)}
		}
	}
}

func (m *Model) syncSlashState() {
	prev := m.slash
	mode, query := parseSlashInput(m.textarea.Value())
	m.slash = slashState{Mode: mode, Query: query, Selected: prev.Selected}

	var options []slashOption
	switch mode {
	case slashModeRoot:
		options = filterSlashOptions([]slashOption{
			{Title: "/connect", Value: "/connect", Description: "Switch provider"},
			{Title: "/model", Value: "/model", Description: "Switch model"},
			{Title: "/sessions", Value: "/sessions", Description: "Open saved sessions"},
			{Title: "/new", Value: "/new", Description: "Create a new session"},
			{Title: "/compact", Value: "/compact", Description: "Compress current context"},
		}, query)
	}

	m.slash.Options = options
	if prev.Mode == mode && len(prev.Options) > 0 && prev.Selected >= 0 && prev.Selected < len(prev.Options) {
		selectedValue := prev.Options[prev.Selected].Value
		for i, option := range options {
			if option.Value == selectedValue {
				m.slash.Selected = i
				break
			}
		}
	}
	if m.slash.Selected >= len(options) {
		m.slash.Selected = len(options) - 1
	}
	if m.slash.Selected < 0 {
		m.slash.Selected = 0
	}
	if mode == slashModeNone {
		m.slash.Selected = 0
	}
	if m.height > 0 {
		m.viewport.Height = max(1, m.height-m.currentComposerHeight()-statusBarHeight-m.commandListHeight())
	}
}

func (m *Model) selectSlash(delta int) {
	if len(m.slash.Options) == 0 {
		return
	}
	m.slash.Selected += delta
	if m.slash.Selected < 0 {
		m.slash.Selected = len(m.slash.Options) - 1
	}
	if m.slash.Selected >= len(m.slash.Options) {
		m.slash.Selected = 0
	}
}

func (m *Model) applySelectedSlashOption() {
	if !m.slash.Visible() {
		return
	}
	option := m.slash.Options[m.slash.Selected]
	switch m.slash.Mode {
	case slashModeRoot:
		m.textarea.SetValue(option.Value + " ")
		m.textarea.CursorEnd()
	}
}

func (m Model) shouldApplySlashSelection(input string) bool {
	if !m.slash.Visible() {
		return false
	}

	trimmed := strings.TrimSpace(input)
	mode, _ := parseSlashInput(trimmed)
	switch mode {
	case slashModeRoot:
		return trimmed != "/new" && trimmed != "/compact" && trimmed != "/connect" && trimmed != "/model" && trimmed != "/sessions"
	default:
		return false
	}
}

func (m *Model) commandListHeight() int {
	if !m.slash.Visible() {
		return 0
	}
	count := len(m.slash.Options)
	if count > 5 {
		count = 5
	}
	return count + 2 + commandListGap
}

func (m Model) renderSlashMenu() string {
	visibleCount := len(m.slash.Options)
	if visibleCount > 5 {
		visibleCount = 5
	}

	var rows []string
	rows = append(rows, slashMenuTitleStyle.Render(m.slashTitle()))
	for i := 0; i < visibleCount; i++ {
		option := m.slash.Options[i]
		style := slashMenuItemStyle
		if i == m.slash.Selected {
			style = slashMenuSelectedStyle
		}
		row := option.Title
		if option.Description != "" {
			row += "  " + slashMenuMetaStyle.Render(option.Description)
		}
		rows = append(rows, style.Width(max(0, m.width-2)).Render(row))
	}

	return lipgloss.NewStyle().MarginBottom(commandListGap).Render(
		slashMenuBoxStyle.Width(max(1, m.width)).Render(strings.Join(rows, "\n")),
	)
}

func (m Model) slashTitle() string {
	return "Commands"
}

func (m *Model) applyEvent(event *amodel.Event) {
	if event.Partial {
		// Streaming thinking delta.
		if event.Message.ReasoningContent != "" {
			if len(m.msgs) > 0 && m.msgs[len(m.msgs)-1].Kind == KindThinking {
				m.msgs[len(m.msgs)-1].Content += event.Message.ReasoningContent
			} else {
				m.msgs = append(m.msgs, ChatMessage{Kind: KindThinking, Content: event.Message.ReasoningContent})
			}
		}
		// Streaming text delta.
		if event.Message.Content != "" {
			if len(m.msgs) > 0 && m.msgs[len(m.msgs)-1].Kind == KindAssistant {
				m.msgs[len(m.msgs)-1].Content += event.Message.Content
			} else {
				m.msgs = append(m.msgs, ChatMessage{Kind: KindAssistant, Content: event.Message.Content})
			}
		}
		return
	}

	switch event.Message.Role {
	case amodel.RoleAssistant:
		if event.Message.Usage != nil {
			m.contextTokens = event.Message.Usage.PromptTokens
		}
		// Finalise the thinking bubble if the complete event carries it.
		if event.Message.ReasoningContent != "" {
			if len(m.msgs) > 0 && m.msgs[len(m.msgs)-1].Kind == KindThinking {
				m.msgs[len(m.msgs)-1].Content = event.Message.ReasoningContent
			} else {
				m.msgs = append(m.msgs, ChatMessage{Kind: KindThinking, Content: event.Message.ReasoningContent})
			}
		}

		if len(event.Message.ToolCalls) > 0 {
			if event.Message.Content != "" {
				if len(m.msgs) > 0 && m.msgs[len(m.msgs)-1].Kind == KindAssistant {
					m.msgs[len(m.msgs)-1].Content = event.Message.Content
				} else {
					m.msgs = append(m.msgs, ChatMessage{Kind: KindAssistant, Content: event.Message.Content})
				}
			} else if len(m.msgs) > 0 && m.msgs[len(m.msgs)-1].Kind == KindAssistant && m.msgs[len(m.msgs)-1].Content == "" {
				m.msgs = m.msgs[:len(m.msgs)-1]
			}
			for _, tc := range event.Message.ToolCalls {
				m.msgs = append(m.msgs, ChatMessage{
					Kind:       KindToolCall,
					ToolName:   tc.Name,
					Args:       tc.Arguments,
					ToolCallID: tc.ID,
				})
			}
		} else {
			if len(m.msgs) > 0 && m.msgs[len(m.msgs)-1].Kind == KindAssistant {
				m.msgs[len(m.msgs)-1].Content = event.Message.Content
			} else {
				m.msgs = append(m.msgs, ChatMessage{Kind: KindAssistant, Content: event.Message.Content})
			}
		}

	case amodel.RoleTool:
		toolName := ""
		for _, cm := range m.msgs {
			if cm.Kind == KindToolCall && cm.ToolCallID == event.Message.ToolCallID {
				toolName = cm.ToolName
				break
			}
		}
		lines := strings.Count(event.Message.Content, "\n") + 1
		m.msgs = append(m.msgs, ChatMessage{
			Kind:       KindToolResult,
			ToolName:   toolName,
			ToolCallID: event.Message.ToolCallID,
			Result:     event.Message.Content,
			Expanded:   lines <= foldThreshold,
		})
	}
}

func (m *Model) moveFocus(delta int) {
	if len(m.msgs) == 0 {
		return
	}
	next := m.focusMsgIdx + delta
	if next < -1 {
		next = -1
	}
	if next >= len(m.msgs) {
		next = len(m.msgs) - 1
	}
	m.focusMsgIdx = next
}

func (m *Model) toggleFocused() {
	if m.focusMsgIdx < 0 || m.focusMsgIdx >= len(m.msgs) {
		return
	}
	if m.msgs[m.focusMsgIdx].Kind == KindToolResult {
		m.msgs[m.focusMsgIdx].Expanded = !m.msgs[m.focusMsgIdx].Expanded
	}
}

func (m *Model) refreshViewport() {
	m.viewport.SetContent(renderMessages(m.msgs, m.focusMsgIdx, m.viewport.Width))
}

// currentComposerHeight returns the number of terminal lines the input area
// currently occupies: 1 separator line + the current textarea height.
func (m Model) currentComposerHeight() int {
	h := m.textarea.Height()
	if h < 1 {
		h = 1
	}
	return 1 + h
}

func (m Model) renderComposer() string {
	sep := inputSeparatorStyle.Render(strings.Repeat("─", m.width))
	input := strings.TrimRight(m.textarea.View(), "\n")
	return lipgloss.JoinVertical(lipgloss.Left, sep, input)
}

func (m Model) renderStatusBar() string {
	var left string

	if m.running {
		if m.escPending {
			left = statusBarStyle.Foreground(lipgloss.Color("214")).Render(" " + m.spinner.View() + "  Esc again to cancel...")
		} else {
			escHint := statusHintStyle.Render("  esc cancel")
			left = statusBarStyle.Render(" "+m.spinner.View()+" "+m.status) + escHint
		}
	} else if m.err != nil {
		left = statusBarStyle.Foreground(lipgloss.Color("9")).Render(" x " + m.err.Error())
	} else {
		left = statusBarStyle.Render(" ")
	}

	model := statusModelStyle.Render(m.runtime.ModelName())
	modeStr := strings.ToUpper(string(m.runtime.Mode()))
	var modeLabel string
	if m.runtime.Mode() == agent.ModePlan {
		modeLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("[" + modeStr + "]")
	} else {
		modeLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("[" + modeStr + "]")
	}
	provider := statusBarStyle.Render(m.runtime.ProviderLabel())
	providerAndMode := modeLabel + " " + provider

	var tokensStr string
	if m.contextTokens >= 1000 {
		tokensStr = fmt.Sprintf("%.1fk tokens", float64(m.contextTokens)/1000.0)
	} else {
		tokensStr = fmt.Sprintf("%d tokens", m.contextTokens)
	}
	tokensStr = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render(tokensStr) + "  "

	hint := statusHintStyle.Render("/ commands  ctrl+c quit ")
	right := providerAndMode + "  " + model + "  " + tokensStr + hint
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := m.width - leftWidth - rightWidth
	if gap < 0 {
		gap = 0
	}
	filler := statusBarStyle.Render(strings.Repeat(" ", gap))
	return left + filler + right
}

func renderMessages(msgs []ChatMessage, focusedIdx, width int) string {
	if len(msgs) == 0 {
		hint := "Start a conversation - ask koda anything about your code. Use / to see commands."
		return emptyStateStyle.Width(width).Align(lipgloss.Center).Padding(2, 0).Render(hint)
	}

	var sb strings.Builder
	sb.WriteByte('\n')
	for i, msg := range msgs {
		sb.WriteString(renderMessage(msg, i == focusedIdx, width))
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func renderMessage(msg ChatMessage, focused bool, width int) string {
	switch msg.Kind {
	case KindUser:
		return renderUserMsg(msg, width)
	case KindAssistant:
		return renderAssistantMsg(msg, width)
	case KindThinking:
		return renderThinkingMsg(msg, width)
	case KindSystem:
		return renderSystemMsg(msg, width)
	case KindToolCall:
		return renderToolCall(msg, focused, width)
	case KindToolResult:
		return renderToolResult(msg, focused, width)
	}
	return ""
}

func renderUserMsg(msg ChatMessage, width int) string {
	contentWidth := width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}
	return userBubbleStyle.Width(contentWidth).Render(msg.Content)
}

func renderAssistantMsg(msg ChatMessage, width int) string {
	contentWidth := width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}
	return assistantBubbleStyle.Width(contentWidth).Render(msg.Content)
}

func renderThinkingMsg(msg ChatMessage, width int) string {
	contentWidth := width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}
	label := thinkingLabelStyle.Render("thinking")
	body := label + "\n" + msg.Content
	return thinkingBubbleStyle.Width(contentWidth).Render(body)
}

func renderSystemMsg(msg ChatMessage, width int) string {
	contentWidth := width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}
	return systemBubbleStyle.Width(contentWidth).Render(msg.Content)
}

func renderToolCall(msg ChatMessage, focused bool, width int) string {
	args := msg.Args
	const maxArgLen = 120
	if len(args) > maxArgLen {
		args = args[:maxArgLen] + "..."
	}
	contentWidth := width - 8
	if contentWidth < 20 {
		contentWidth = 20
	}
	style := toolCallCardStyle
	if focused {
		style = toolCallCardFocusedStyle
	}
	prefix := toolCallPrefixStyle.Render("tool ")
	call := toolCallArgsStyle.Render(fmt.Sprintf("%s(%s)", msg.ToolName, args))
	return style.Width(contentWidth).Render(prefix + call)
}

func renderToolResult(msg ChatMessage, focused bool, width int) string {
	lineCount := strings.Count(msg.Result, "\n") + 1
	style := toolResultCardStyle
	if focused {
		style = toolResultCardFocusedStyle
	}
	contentWidth := width - 8
	if contentWidth < 20 {
		contentWidth = 20
	}
	if !msg.Expanded {
		label := fmt.Sprintf("%d lines - press ] then x to expand", lineCount)
		if msg.ToolName != "" {
			label = fmt.Sprintf("%s output - %s", msg.ToolName, label)
		}
		return style.Width(contentWidth).Render(toolResultMetaStyle.Render(label))
	}
	var parts []string
	if msg.ToolName != "" {
		parts = append(parts, toolResultMetaStyle.Render(fmt.Sprintf("%s output - %d lines", msg.ToolName, lineCount)))
	}
	parts = append(parts, msg.Result)
	return style.Width(contentWidth).Render(strings.Join(parts, "\n"))
}

func chatMessagesFromSession(msgs []*message.Message) []ChatMessage {
	result := make([]ChatMessage, 0, len(msgs))
	toolNames := make(map[string]string)
	for _, msg := range msgs {
		switch msg.Role {
		case string(amodel.RoleUser):
			result = append(result, ChatMessage{Kind: KindUser, Content: msg.Content})
		case string(amodel.RoleAssistant):
			if msg.ReasoningContent != "" {
				result = append(result, ChatMessage{Kind: KindThinking, Content: msg.ReasoningContent})
			}
			if msg.Content != "" {
				result = append(result, ChatMessage{Kind: KindAssistant, Content: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				toolNames[tc.ID] = tc.Name
				result = append(result, ChatMessage{Kind: KindToolCall, ToolName: tc.Name, Args: tc.Arguments, ToolCallID: tc.ID})
			}
		case string(amodel.RoleTool):
			lines := strings.Count(msg.Content, "\n") + 1
			result = append(result, ChatMessage{Kind: KindToolResult, ToolName: toolNames[msg.ToolCallID], ToolCallID: msg.ToolCallID, Result: msg.Content, Expanded: lines <= foldThreshold})
		case string(amodel.RoleSystem):
			result = append(result, ChatMessage{Kind: KindSystem, Content: msg.Content})
		}
	}
	return result
}

func formatSessionLabel(sess agent.SessionMeta) string {
	return fmt.Sprintf("%s  #%d", sess.Title, sess.SessionID)
}

func formatSessionMeta(sess agent.SessionMeta) string {
	dir := strings.TrimSpace(sess.WorkDir)
	if dir == "" {
		dir = "."
	} else {
		dir = filepath.Base(dir)
	}
	updated := "unknown time"
	if sess.UpdatedAt > 0 {
		updated = time.UnixMilli(sess.UpdatedAt).Format("2006-01-02 15:04")
	}
	return fmt.Sprintf("%s - %s", dir, updated)
}
