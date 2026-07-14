package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/permission"
	kodatools "github.com/soasurs/koda/internal/tools"
)

type namespacedTool struct {
	delegate     tool.Tool
	definition   tool.Definition
	originalName string
}

type approvalTool struct {
	delegate   tool.Tool
	serverID   string
	serverName string
	authorizer kodatools.Authorizer
}

func newApprovalTool(delegate tool.Tool, serverID, serverName string, authorizer kodatools.Authorizer) tool.Tool {
	return &approvalTool{delegate: delegate, serverID: serverID, serverName: serverName, authorizer: authorizer}
}

func (t *approvalTool) Definition() tool.Definition {
	return t.delegate.Definition()
}

func (t *approvalTool) Run(ctx context.Context, call tool.Call) (*tool.Result, error) {
	if t.authorizer == nil {
		return nil, tool.NewHandledError(kodatools.ErrApprovalRequired.Error())
	}
	err := t.authorizer.Authorize(ctx, kodatools.Approval{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Arguments:  append([]byte(nil), call.Arguments...),
		Kind:       permission.KindMCP,
		Scope:      permission.ScopeGlobal,
		Summary:    fmt.Sprintf("run MCP tool %s from %s (%s)", call.Name, t.serverName, t.serverID),
	})
	if errors.Is(err, kodatools.ErrApprovalRejected) {
		return nil, tool.NewHandledError("tool approval rejected")
	}
	if err != nil {
		return nil, err
	}
	return t.delegate.Run(ctx, call)
}

func newNamespacedTool(delegate tool.Tool, name, originalName, serverName string) tool.Tool {
	definition := delegate.Definition()
	definition.Name = name
	if definition.Description == "" {
		definition.Description = fmt.Sprintf("MCP tool %s from %s", originalName, serverName)
	} else {
		definition.Description = fmt.Sprintf("[%s MCP] %s", serverName, definition.Description)
	}
	return &namespacedTool{delegate: delegate, definition: definition, originalName: originalName}
}

func (t *namespacedTool) Definition() tool.Definition {
	return t.definition
}

func (t *namespacedTool) Run(ctx context.Context, call tool.Call) (*tool.Result, error) {
	call.Name = t.originalName
	result, err := t.delegate.Run(ctx, call)
	if err != nil || result == nil {
		return result, err
	}
	result = result.Clone()
	result.Content = truncateResult(result.Content)
	if len(result.StructuredContent) > defaultMaxResultBytes {
		result.StructuredContent = nil
		if result.Content == "" {
			result.Content = "MCP structured result exceeded the Koda output limit and was omitted"
		}
	}
	return result, nil
}

func truncateResult(value string) string {
	if len(value) <= defaultMaxResultBytes {
		return value
	}
	prefix := value[:defaultMaxResultBytes]
	for !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}
	return prefix + "\n...[MCP result truncated by Koda]"
}

func exposedToolName(serverID, original string) (string, error) {
	original = strings.TrimSpace(original)
	if original == "" {
		return "", fmt.Errorf("tool name must not be empty")
	}
	var builder strings.Builder
	builder.WriteString("mcp__")
	builder.WriteString(serverID)
	builder.WriteString("__")
	for _, value := range original {
		if value <= unicode.MaxASCII && (unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_' || value == '-') {
			builder.WriteRune(value)
		} else {
			builder.WriteByte('_')
		}
	}
	name := builder.String()
	if len(name) > 64 {
		return "", fmt.Errorf("namespaced tool name %q exceeds 64 bytes", name)
	}
	return name, nil
}

var _ tool.Tool = (*namespacedTool)(nil)
var _ tool.Tool = (*approvalTool)(nil)
