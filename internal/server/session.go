package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"connectrpc.com/connect"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/permission"
	"github.com/soasurs/koda/internal/store"
	"google.golang.org/protobuf/proto"
)

// CreateSession creates a new session with a validated execution configuration.
func (h *Handler) CreateSession(ctx context.Context, request *v1.CreateSessionRequest) (*v1.CreateSessionResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("create session request must not be nil"))
	}
	workdir, err := normalizeWorkdir(request.GetWorkdir())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	configuration, err := h.validateSessionConfiguration(ctx, request.GetProviderId(), request.GetModelId(), request.GetReasoningEffort())
	if err != nil {
		return nil, err
	}
	fileAccess, err := fileAccessFromProto(request.GetFileAccess())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	shellAccess, err := shellAccessFromProto(request.GetShellAccess())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	id, err := h.newSessionID()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("generate session ID"))
	}
	session, err := h.store.CreateSession(ctx, store.CreateSessionParams{
		ID:              id,
		Workdir:         workdir,
		ProviderID:      configuration.providerID,
		ModelID:         configuration.modelID,
		ReasoningEffort: configuration.reasoningEffort,
		FileAccess:      fileAccess,
		ShellAccess:     shellAccess,
	})
	if err != nil {
		return nil, sessionError(err)
	}
	return v1.CreateSessionResponse_builder{Session: sessionToProto(session)}.Build(), nil
}

// GetSession returns one session by ID.
func (h *Handler) GetSession(ctx context.Context, request *v1.GetSessionRequest) (*v1.GetSessionResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("get session request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.GetSessionId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	session, err := h.store.GetSession(ctx, id)
	if err != nil {
		return nil, sessionError(err)
	}
	return v1.GetSessionResponse_builder{Session: sessionToProto(session)}.Build(), nil
}

// ListSessions returns a page of sessions ordered by last update time.
func (h *Handler) ListSessions(ctx context.Context, request *v1.ListSessionsRequest) (*v1.ListSessionsResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("list sessions request must not be nil"))
	}
	if request.GetLimit() < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session list limit must not be negative"))
	}
	if request.GetOffset() < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session list offset must not be negative"))
	}
	sessions, total, err := h.store.ListSessions(ctx, store.ListSessionsParams{
		Limit:  int(request.GetLimit()),
		Offset: request.GetOffset(),
	})
	if err != nil {
		return nil, sessionError(err)
	}
	return v1.ListSessionsResponse_builder{
		Sessions: sessionsToProto(sessions),
		Total:    proto.Int64(total),
	}.Build(), nil
}

// UpdateSession applies one partial configuration update to a session.
func (h *Handler) UpdateSession(ctx context.Context, request *v1.UpdateSessionRequest) (*v1.UpdateSessionResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("update session request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.GetSessionId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	unlock, err := h.store.LockRun(ctx, id)
	if err != nil {
		return nil, sessionError(err)
	}
	defer unlock()

	current, err := h.store.GetSession(ctx, id)
	if err != nil {
		return nil, sessionError(err)
	}
	params, candidate, validateConfiguration, err := updateSessionParams(request, current)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if validateConfiguration {
		configuration, err := h.validateSessionConfiguration(
			ctx,
			candidate.ProviderID,
			candidate.ModelID,
			candidate.ReasoningEffort,
		)
		if err != nil {
			return nil, err
		}
		params.ProviderID = new(configuration.providerID)
		params.ModelID = new(configuration.modelID)
		params.ReasoningEffort = new(configuration.reasoningEffort)
	}
	session, err := h.store.UpdateSession(ctx, id, params)
	if err != nil {
		return nil, sessionError(err)
	}
	return v1.UpdateSessionResponse_builder{Session: sessionToProto(session)}.Build(), nil
}

// DeleteSession deletes one session and hides its ADK history.
func (h *Handler) DeleteSession(ctx context.Context, request *v1.DeleteSessionRequest) (*v1.DeleteSessionResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("delete session request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.GetSessionId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.store.DeleteSession(ctx, id); err != nil {
		return nil, sessionError(err)
	}
	return v1.DeleteSessionResponse_builder{}.Build(), nil
}

type sessionConfiguration struct {
	providerID      string
	modelID         string
	reasoningEffort string
}

func (h *Handler) validateSessionConfiguration(
	ctx context.Context,
	providerID string,
	modelID string,
	reasoningEffort string,
) (sessionConfiguration, error) {
	providerID, err := providerIDFromRequest(providerID)
	if err != nil {
		return sessionConfiguration{}, connect.NewError(connect.CodeInvalidArgument, err)
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return sessionConfiguration{}, connect.NewError(connect.CodeInvalidArgument, errors.New("model ID must not be empty"))
	}
	reasoningEffort = strings.TrimSpace(reasoningEffort)
	catalog, err := h.catalog.List(ctx, providerID)
	if err != nil {
		return sessionConfiguration{}, providerError(err)
	}
	for _, model := range catalog.Models {
		if model.ID != modelID {
			continue
		}
		if reasoningEffort != "" && !slices.Contains(model.ReasoningEfforts, reasoningEffort) {
			return sessionConfiguration{}, connect.NewError(
				connect.CodeInvalidArgument,
				fmt.Errorf("model %q does not support reasoning effort %q", modelID, reasoningEffort),
			)
		}
		return sessionConfiguration{
			providerID:      providerID,
			modelID:         modelID,
			reasoningEffort: reasoningEffort,
		}, nil
	}
	return sessionConfiguration{}, connect.NewError(
		connect.CodeInvalidArgument,
		fmt.Errorf("provider %q does not expose model %q", providerID, modelID),
	)
}

func updateSessionParams(request *v1.UpdateSessionRequest, current store.Session) (store.UpdateSessionParams, store.Session, bool, error) {
	candidate := current
	params := store.UpdateSessionParams{}
	if request.HasTitle() {
		value := strings.TrimSpace(request.GetTitle())
		params.Title = &value
		candidate.Title = value
	}
	if request.HasWorkdir() {
		value, err := normalizeWorkdir(request.GetWorkdir())
		if err != nil {
			return store.UpdateSessionParams{}, store.Session{}, false, err
		}
		params.Workdir = &value
		candidate.Workdir = value
	}
	validateConfiguration := request.HasProviderId() || request.HasModelId() || request.HasReasoningEffort()
	if request.HasProviderId() {
		value, err := providerIDFromRequest(request.GetProviderId())
		if err != nil {
			return store.UpdateSessionParams{}, store.Session{}, false, err
		}
		params.ProviderID = &value
		candidate.ProviderID = value
	}
	if request.HasModelId() {
		value := strings.TrimSpace(request.GetModelId())
		if value == "" {
			return store.UpdateSessionParams{}, store.Session{}, false, errors.New("model ID must not be empty")
		}
		params.ModelID = &value
		candidate.ModelID = value
	}
	if request.HasReasoningEffort() {
		value := strings.TrimSpace(request.GetReasoningEffort())
		params.ReasoningEffort = &value
		candidate.ReasoningEffort = value
	}
	if request.HasFileAccess() {
		value, err := fileAccessFromProto(request.GetFileAccess())
		if err != nil {
			return store.UpdateSessionParams{}, store.Session{}, false, err
		}
		params.FileAccess = &value
		candidate.FileAccess = value
	}
	if request.HasShellAccess() {
		value, err := shellAccessFromProto(request.GetShellAccess())
		if err != nil {
			return store.UpdateSessionParams{}, store.Session{}, false, err
		}
		params.ShellAccess = &value
		candidate.ShellAccess = value
	}
	return params, candidate, validateConfiguration, nil
}

func normalizeWorkdir(workdir string) (string, error) {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return "", errors.New("workdir must not be empty")
	}
	abs, err := filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("resolve workdir: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat workdir: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("workdir must be a directory")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve workdir symlinks: %w", err)
	}
	return filepath.Clean(resolved), nil
}

func sessionIDFromRequest(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("session ID must not be empty")
	}
	return id, nil
}

func sessionToProto(session store.Session) *v1.Session {
	return v1.Session_builder{
		Id:              proto.String(session.ID),
		Title:           proto.String(session.Title),
		Workdir:         proto.String(session.Workdir),
		ProviderId:      proto.String(session.ProviderID),
		ModelId:         proto.String(session.ModelID),
		ReasoningEffort: proto.String(session.ReasoningEffort),
		FileAccess:      fileAccessToProto(session.FileAccess).Enum(),
		ShellAccess:     shellAccessToProto(session.ShellAccess).Enum(),
		CreatedAt:       proto.Int64(session.CreatedAt.UnixMilli()),
		UpdatedAt:       proto.Int64(session.UpdatedAt.UnixMilli()),
		EventCount:      proto.Int64(session.EventCount),
	}.Build()
}

func sessionsToProto(sessions []store.Session) []*v1.Session {
	result := make([]*v1.Session, len(sessions))
	for i, session := range sessions {
		result[i] = sessionToProto(session)
	}
	return result
}

func fileAccessFromProto(value v1.FileAccess) (permission.FileAccess, error) {
	switch value {
	case v1.FileAccess_FILE_ACCESS_UNSPECIFIED, v1.FileAccess_FILE_ACCESS_WORKSPACE_READ:
		return permission.FileAccessWorkspaceRead, nil
	case v1.FileAccess_FILE_ACCESS_WORKSPACE_WRITE:
		return permission.FileAccessWorkspaceWrite, nil
	case v1.FileAccess_FILE_ACCESS_UNRESTRICTED:
		return permission.FileAccessUnrestricted, nil
	default:
		return "", fmt.Errorf("invalid file access %q", value)
	}
}

func fileAccessToProto(value permission.FileAccess) v1.FileAccess {
	switch value {
	case permission.FileAccessWorkspaceWrite:
		return v1.FileAccess_FILE_ACCESS_WORKSPACE_WRITE
	case permission.FileAccessUnrestricted:
		return v1.FileAccess_FILE_ACCESS_UNRESTRICTED
	default:
		return v1.FileAccess_FILE_ACCESS_WORKSPACE_READ
	}
}

func shellAccessFromProto(value v1.ShellAccess) (permission.ShellAccess, error) {
	switch value {
	case v1.ShellAccess_SHELL_ACCESS_UNSPECIFIED, v1.ShellAccess_SHELL_ACCESS_APPROVAL_REQUIRED:
		return permission.ShellAccessApprovalRequired, nil
	case v1.ShellAccess_SHELL_ACCESS_UNRESTRICTED:
		return permission.ShellAccessUnrestricted, nil
	default:
		return "", fmt.Errorf("invalid shell access %q", value)
	}
}

func shellAccessToProto(value permission.ShellAccess) v1.ShellAccess {
	if value == permission.ShellAccessUnrestricted {
		return v1.ShellAccess_SHELL_ACCESS_UNRESTRICTED
	}
	return v1.ShellAccess_SHELL_ACCESS_APPROVAL_REQUIRED
}
