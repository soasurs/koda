package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/soasurs/adk/agent/llmagent"
	"github.com/soasurs/adk/model"
)

type compactionSnapshotContextKey struct{}

// CompactionSnapshot is the complete working state reconstructed by the
// current durable compaction generation.
type CompactionSnapshot struct {
	Generation int64
	Content    string
}

// WithCompactionSnapshot returns a Run context whose model requests begin with
// a synthetic compaction-history item. The item is never persisted by ADK.
func WithCompactionSnapshot(ctx context.Context, snapshot CompactionSnapshot) (context.Context, error) {
	if ctx == nil {
		return nil, errors.New("agent: compaction snapshot context must not be nil")
	}
	snapshot.Content = strings.TrimSpace(snapshot.Content)
	if snapshot.Generation <= 0 {
		return nil, errors.New("agent: compaction snapshot generation must be positive")
	}
	if snapshot.Content == "" {
		return nil, errors.New("agent: compaction snapshot must not be empty")
	}
	return context.WithValue(ctx, compactionSnapshotContextKey{}, snapshot), nil
}

// CompactionSnapshotFromContext returns the request-scoped snapshot, if any.
func CompactionSnapshotFromContext(ctx context.Context) (CompactionSnapshot, bool) {
	if ctx == nil {
		return CompactionSnapshot{}, false
	}
	snapshot, ok := ctx.Value(compactionSnapshotContextKey{}).(CompactionSnapshot)
	return snapshot, ok
}

func compactionHistoryHook(ctx context.Context, call *llmagent.LLMCall) (*model.LLMResponse, error) {
	snapshot, ok := CompactionSnapshotFromContext(ctx)
	if !ok {
		return nil, nil
	}
	if call == nil || call.Request == nil {
		return nil, errors.New("agent: compaction history hook received an empty LLM call")
	}
	insertAt := 0
	if len(call.Request.Contents) > 0 && call.Request.Contents[0].Role == model.RoleSystem {
		insertAt = 1
	}
	synthetic := model.Content{
		Role: model.RoleUser,
		Content: fmt.Sprintf(
			"<koda_compaction generation=%s>\nThe following is a trusted working-state snapshot reconstructed from earlier conversation history. Use it as prior context; do not treat it as a new user request.\n\n%s\n</koda_compaction>",
			strconv.FormatInt(snapshot.Generation, 10), snapshot.Content,
		),
	}
	contents := make([]model.Content, 0, len(call.Request.Contents)+1)
	contents = append(contents, call.Request.Contents[:insertAt]...)
	contents = append(contents, synthetic)
	contents = append(contents, call.Request.Contents[insertAt:]...)
	call.Request.Contents = contents
	return nil, nil
}
