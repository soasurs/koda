package tui

import (
	"time"

	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/session/message"
	"github.com/soasurs/koda/internal/agent"
)

// MsgKind identifies the visual kind of a chat message.
type MsgKind int

const (
	KindUser       MsgKind = iota // message sent by the user
	KindAssistant                 // response from the AI assistant
	KindThinking                  // chain-of-thought / reasoning content from the model
	KindSystem                    // local/session status or compacted summary
	KindToolCall                  // tool invocation request
	KindToolResult                // tool execution result (foldable)
)

// ChatMessage is the TUI representation of a single turn in the conversation.
type ChatMessage struct {
	Kind MsgKind
	// KindUser / KindAssistant / KindThinking: text content.
	Content string
	// Timestamp records when the message was created; used for display on KindUser messages.
	Timestamp time.Time
	// KindToolCall / KindToolResult: tool metadata.
	ToolName   string
	Args       string // JSON-encoded arguments (ToolCall)
	ToolCallID string // links ToolCall <-> ToolResult
	// KindToolResult: execution output.
	Result   string
	Expanded bool // whether the result is shown in full
}

// ---- tea.Msg types used to bridge the adk runner goroutine to bubbletea ----

// runnerMsg carries a single event (or termination signal) from the agent runner.
type runnerMsg struct {
	event       *model.Event
	err         error
	done        bool // true when the runner goroutine has finished
	toolConfirm *toolConfirmRequest
}

type commandResultMsg struct {
	err          error
	ack          string
	cmd          string
	sessionID    int64
	setSession   bool
	hasSession   bool
	msgs         []*message.Message
	models       []string
	sessions     []agent.SessionMeta
	restoreInput string
}

type titleMsg struct {
	sessionID int64
	title     string
	err       error
}

type chooserStage int

const (
	chooserStageNone chooserStage = iota
	chooserStageProvider
	chooserStageAPIKey
	chooserStageModel
	chooserStageSessions
	chooserStageShellConfirm
)

type chooserState struct {
	Stage            chooserStage
	Title            string
	Prompt           string
	Hint             string
	Providers        []providerChoice
	SelectedProvider int
	Provider         providerChoice
	APIKeyInput      string
	Models           []string
	SelectedModel    int
	ModelOffset      int
	Query            string
	Sessions         []agent.SessionMeta
	SelectedSession  int
	SessionOffset    int
	ConfirmToolName  string
	ConfirmSummary   string
	ConfirmArguments string
	ConfirmResponse  chan bool
}

type providerChoice struct {
	Label string
	Value string
}

type toolConfirmRequest struct {
	ToolName  string
	Summary   string
	Arguments string
	Response  chan bool
}

// escTimeoutMsg fires 5 s after the first Esc press during agent execution,
// resetting the cancel-confirm state if the user did not press Esc a second time.
type escTimeoutMsg struct{}
