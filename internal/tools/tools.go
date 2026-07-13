// Package tools implements Koda's workspace-aware coding tools.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/logging"
	"github.com/soasurs/koda/internal/permission"
)

const (
	defaultCommandTimeout = 30 * time.Second
	defaultMaxChars       = 32 * 1024
	defaultMaxEntries     = 200
	defaultMaxResults     = 200
	defaultDiffContext    = 3
)

var (
	// ErrApprovalRejected indicates that the user rejected a tool operation.
	ErrApprovalRejected = errors.New("tool approval rejected")
	// ErrApprovalRequired indicates that an operation needs an authorizer but
	// none was configured for this tool set.
	ErrApprovalRequired = errors.New("tool approval required")
)

// Config controls the tools constructed for one session and one agent mode.
type Config struct {
	// Workdir is the session workspace used to resolve relative paths.
	Workdir string
	// FileAccess controls automatic filesystem access. An empty value uses the
	// least permissive valid level.
	FileAccess permission.FileAccess
	// ShellAccess controls automatic shell execution. An empty value requires
	// approval for every command.
	ShellAccess permission.ShellAccess
	// Authorizer receives operations that are not automatically permitted. It
	// may block until the user accepts or rejects the request.
	Authorizer Authorizer
	// Questioner publishes ask_questions prompts and blocks until the frontend
	// submits answers or cancels the prompt.
	Questioner Questioner
	// CommandTimeout limits rg, git, and shell executions. Zero uses a safe
	// default.
	CommandTimeout time.Duration
	// Logger receives tool execution diagnostics. Tool inputs and outputs are
	// never logged.
	Logger *slog.Logger
}

// Authorizer confirms an operation that is not covered by the session's
// automatic access settings. Returning ErrApprovalRejected creates a
// model-visible handled error; context cancellation remains terminal.
type Authorizer interface {
	Authorize(context.Context, Approval) error
}

// Approval describes an operation awaiting a user decision.
type Approval struct {
	// ToolCallID correlates this approval with the provider tool call that
	// requested the operation.
	ToolCallID string
	// ToolName identifies the tool that requested the operation.
	ToolName string
	// Arguments contains the exact JSON arguments supplied by the model.
	Arguments   json.RawMessage
	Kind        permission.Kind
	Scope       permission.Scope
	TargetPaths []string
	Summary     string
	FileChanges []FileChange
}

// FileChange is a display-oriented change to one file. It mirrors the public
// Proto diff types without making core tools depend on generated bindings.
type FileChange struct {
	Path      string         `json:"path"`
	Kind      FileChangeKind `json:"kind"`
	Hunks     []DiffHunk     `json:"hunks,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
}

// FileChangeKind identifies the effect represented by a FileChange.
type FileChangeKind string

const (
	// FileChangeCreate identifies a newly created file.
	FileChangeCreate FileChangeKind = "create"
	// FileChangeUpdate identifies an existing file update.
	FileChangeUpdate FileChangeKind = "update"
	// FileChangeDelete identifies a deleted file.
	FileChangeDelete FileChangeKind = "delete"
)

// DiffHunk groups changed lines with surrounding context.
type DiffHunk struct {
	OldStart int        `json:"old_start"`
	NewStart int        `json:"new_start"`
	Lines    []DiffLine `json:"lines"`
}

// DiffLine is one display line in a DiffHunk.
type DiffLine struct {
	Kind    DiffLineKind `json:"kind"`
	OldLine int          `json:"old_line,omitempty"`
	NewLine int          `json:"new_line,omitempty"`
	Content string       `json:"content"`
}

// DiffLineKind identifies how a DiffLine participates in a hunk.
type DiffLineKind string

const (
	// DiffLineContext identifies unchanged surrounding content.
	DiffLineContext DiffLineKind = "context"
	// DiffLineAdded identifies a line introduced by the change.
	DiffLineAdded DiffLineKind = "added"
	// DiffLineRemoved identifies a line removed by the change.
	DiffLineRemoved DiffLineKind = "removed"
)

type service struct {
	resolver       resolver
	fileAccess     permission.FileAccess
	shellAccess    permission.ShellAccess
	authorizer     Authorizer
	questioner     Questioner
	commandTimeout time.Duration
	logger         *slog.Logger
}

func newService(config Config) (service, error) {
	workdir := strings.TrimSpace(config.Workdir)
	if workdir == "" {
		return service{}, errors.New("tools: workdir must not be empty")
	}
	abs, err := filepath.Abs(workdir)
	if err != nil {
		return service{}, fmt.Errorf("tools: resolve workdir: %w", err)
	}
	workspace, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return service{}, fmt.Errorf("tools: resolve workdir symlinks: %w", err)
	}
	if config.FileAccess == "" {
		config.FileAccess = permission.DefaultFileAccess
	}
	if !config.FileAccess.Valid() {
		return service{}, fmt.Errorf("tools: invalid file access %q", config.FileAccess)
	}
	if config.ShellAccess == "" {
		config.ShellAccess = permission.DefaultShellAccess
	}
	if !config.ShellAccess.Valid() {
		return service{}, fmt.Errorf("tools: invalid shell access %q", config.ShellAccess)
	}
	if config.CommandTimeout <= 0 {
		config.CommandTimeout = defaultCommandTimeout
	}
	return service{
		resolver:       resolver{workspace: filepath.Clean(workspace)},
		fileAccess:     config.FileAccess,
		shellAccess:    config.ShellAccess,
		authorizer:     config.Authorizer,
		questioner:     config.Questioner,
		commandTimeout: config.CommandTimeout,
		logger:         logging.OrDiscard(config.Logger),
	}, nil
}

func (s service) authorize(ctx context.Context, kind permission.Kind, scope permission.Scope, targets []string, summary string, changes []FileChange) error {
	if !permission.RequiresApproval(s.fileAccess, s.shellAccess, kind, scope) {
		return nil
	}
	if s.authorizer == nil {
		return tool.NewHandledError(ErrApprovalRequired.Error())
	}
	approval := Approval{
		Kind:        kind,
		Scope:       scope,
		TargetPaths: append([]string(nil), targets...),
		Summary:     summary,
		FileChanges: append([]FileChange(nil), changes...),
	}
	if call, ok := toolCallFromContext(ctx); ok {
		approval.ToolCallID = call.ID
		approval.ToolName = call.Name
		approval.Arguments = append(json.RawMessage(nil), call.Arguments...)
	}
	err := s.authorizer.Authorize(ctx, approval)
	if errors.Is(err, ErrApprovalRejected) {
		return tool.NewHandledError("tool approval rejected")
	}
	return err
}

// NewReadOnly constructs the tools available in Plan mode.
func NewReadOnly(config Config) ([]tool.Tool, error) {
	s, err := newService(config)
	if err != nil {
		return nil, err
	}
	values, err := s.readOnlyTools()
	if err != nil {
		return nil, err
	}
	runShell, err := s.newPlanShellTool()
	if err != nil {
		return nil, err
	}
	return withToolCallContext(append(values, runShell)), nil
}

// NewBuild constructs the tools available in Build mode.
func NewBuild(config Config) ([]tool.Tool, error) {
	s, err := newService(config)
	if err != nil {
		return nil, err
	}
	tools, err := s.readOnlyTools()
	if err != nil {
		return nil, err
	}
	writeFile, err := s.newWriteFileTool()
	if err != nil {
		return nil, err
	}
	createFile, err := s.newCreateFileTool()
	if err != nil {
		return nil, err
	}
	editFile, err := s.newEditFileTool()
	if err != nil {
		return nil, err
	}
	runShell, err := s.newRunShellTool()
	if err != nil {
		return nil, err
	}
	return withToolCallContext(append(tools, writeFile, createFile, editFile, runShell)), nil
}

func (s service) readOnlyTools() ([]tool.Tool, error) {
	readFile, err := s.newReadFileTool()
	if err != nil {
		return nil, err
	}
	listDirectory, err := s.newListDirectoryTool()
	if err != nil {
		return nil, err
	}
	searchText, err := s.newSearchTextTool()
	if err != nil {
		return nil, err
	}
	findFiles, err := s.newFindFilesTool()
	if err != nil {
		return nil, err
	}
	askQuestions, err := s.newAskQuestionsTool()
	if err != nil {
		return nil, err
	}
	return []tool.Tool{readFile, listDirectory, searchText, findFiles, askQuestions}, nil
}

type toolCallContextKey struct{}

type toolCallContextTool struct {
	tool.Tool
}

func (t toolCallContextTool) Run(ctx context.Context, call tool.Call) (*tool.Result, error) {
	return t.Tool.Run(context.WithValue(ctx, toolCallContextKey{}, cloneToolCall(call)), call)
}

func withToolCallContext(values []tool.Tool) []tool.Tool {
	result := make([]tool.Tool, len(values))
	for index, value := range values {
		result[index] = toolCallContextTool{Tool: value}
	}
	return result
}

func toolCallFromContext(ctx context.Context) (tool.Call, bool) {
	if ctx == nil {
		return tool.Call{}, false
	}
	call, ok := ctx.Value(toolCallContextKey{}).(tool.Call)
	return call, ok
}

func cloneToolCall(call tool.Call) tool.Call {
	call.Arguments = append(json.RawMessage(nil), call.Arguments...)
	return call
}
