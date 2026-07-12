package provider

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestCatalogUsesBundledModelsForDefaultBuiltin(t *testing.T) {
	r := openTestRegistry(t)
	catalog, err := NewCatalog(r, fakeDiscoverer{})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	got, err := catalog.List(t.Context(), "openai-responses")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !got.RefreshedAt.IsZero() {
		t.Fatalf("RefreshedAt = %v, want zero", got.RefreshedAt)
	}
	model, ok := findModel(got.Models, "gpt-5.6")
	if !ok {
		t.Fatalf("bundled catalog does not contain gpt-5.6: %+v", got.Models)
	}
	if !slices.Contains(model.ReasoningEfforts, "max") {
		t.Fatalf("gpt-5.6 reasoning efforts = %v, want max", model.ReasoningEfforts)
	}
}

func TestCatalogDeepSeekReasoningEfforts(t *testing.T) {
	r := openTestRegistry(t)
	catalog, err := NewCatalog(r, fakeDiscoverer{})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	got, err := catalog.List(t.Context(), "deepseek")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for _, id := range []string{"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-reasoner"} {
		model, ok := findModel(got.Models, id)
		if !ok {
			t.Fatalf("bundled catalog does not contain %q", id)
		}
		if !slices.Equal(model.ReasoningEfforts, []string{"high", "max"}) || model.DefaultReasoningEffort != "high" {
			t.Fatalf("%s reasoning configuration = %+v", id, model)
		}
	}
	chat, ok := findModel(got.Models, "deepseek-chat")
	if !ok {
		t.Fatal("bundled catalog does not contain deepseek-chat")
	}
	if len(chat.ReasoningEfforts) != 0 || chat.DefaultReasoningEffort != "" {
		t.Fatalf("deepseek-chat reasoning configuration = %+v, want non-thinking", chat)
	}
}

func TestCatalogCustomProviderWithoutSnapshotUsesOnlyOverrides(t *testing.T) {
	r := openTestRegistry(t)
	if _, err := r.Save(t.Context(), Provider{
		ID:      "custom",
		Name:    "Custom",
		Type:    TypeOpenAIResponses,
		BaseURL: "https://models.example/v1",
		ModelOverrides: []Model{{
			ID: "private-model",
		}},
	}, nil); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	catalog, err := NewCatalog(r, fakeDiscoverer{})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	got, err := catalog.List(t.Context(), "custom")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "private-model" || got.Models[0].Name != "private-model" {
		t.Fatalf("List() = %+v", got)
	}
}

func TestCatalogRefreshEnrichesPersistsAndPreservesProviderRevision(t *testing.T) {
	r := openTestRegistry(t)
	p, err := r.Get(t.Context(), "anthropic")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	p.ModelOverrides = []Model{{ID: "claude-sonnet-4-6", Name: "Preferred Sonnet"}}
	p, err = r.Save(t.Context(), p, nil)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	beforeRevision := p.Revision()

	discoverer := fakeDiscoverer{models: []Model{
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet from API"},
		{ID: "claude-new", Name: "Claude New"},
	}}
	catalog, err := NewCatalog(r, discoverer)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	refreshedAt := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	catalog.now = func() time.Time { return refreshedAt }

	got, err := catalog.Refresh(t.Context(), "anthropic")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if got.RefreshedAt != refreshedAt || len(got.Models) != 2 {
		t.Fatalf("Refresh() = %+v", got)
	}
	sonnet, ok := findModel(got.Models, "claude-sonnet-4-6")
	if !ok {
		t.Fatal("Refresh() omitted claude-sonnet-4-6")
	}
	if sonnet.Name != "Preferred Sonnet" || !slices.Contains(sonnet.ReasoningEfforts, "max") {
		t.Fatalf("enriched Sonnet = %+v", sonnet)
	}
	afterProvider, err := r.Get(t.Context(), "anthropic")
	if err != nil {
		t.Fatalf("Get() after refresh error = %v", err)
	}
	if afterProvider.Revision() != beforeRevision {
		t.Fatalf("provider revision changed from %d to %d", beforeRevision, afterProvider.Revision())
	}

	reopened, err := Open(r.path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	reopenedCatalog, err := NewCatalog(reopened, fakeDiscoverer{})
	if err != nil {
		t.Fatalf("NewCatalog(reopened) error = %v", err)
	}
	persisted, err := reopenedCatalog.List(t.Context(), "anthropic")
	if err != nil {
		t.Fatalf("List(reopened) error = %v", err)
	}
	if persisted.RefreshedAt != refreshedAt || len(persisted.Models) != 2 {
		t.Fatalf("persisted catalog = %+v", persisted)
	}
}

func TestCatalogFailedRefreshPreservesPreviousSnapshot(t *testing.T) {
	r := openTestRegistry(t)
	p, err := r.Get(t.Context(), "deepseek")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	refreshedAt := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	if _, err := r.SetModelSnapshot(t.Context(), "deepseek", p.Revision(), ModelSnapshot{
		Models:      []Model{{ID: "deepseek-chat"}},
		RefreshedAt: refreshedAt,
	}); err != nil {
		t.Fatalf("SetModelSnapshot() error = %v", err)
	}
	catalog, err := NewCatalog(r, fakeDiscoverer{err: errors.New("offline")})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	if _, err := catalog.Refresh(t.Context(), "deepseek"); err == nil {
		t.Fatal("Refresh() error = nil, want discovery error")
	}
	got, err := catalog.List(t.Context(), "deepseek")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got.RefreshedAt != refreshedAt || len(got.Models) != 1 || got.Models[0].ID != "deepseek-chat" {
		t.Fatalf("List() after failed refresh = %+v", got)
	}
}

func TestCatalogRefreshRejectsSnapshotFromStaleProviderConfiguration(t *testing.T) {
	r := openTestRegistry(t)
	key := "old-key"
	p, err := r.Save(t.Context(), Provider{
		ID:      "custom",
		Name:    "Custom",
		Type:    TypeOpenAIResponses,
		BaseURL: "https://old.example/v1",
	}, &key)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	discoverer := &blockingDiscoverer{
		started: make(chan struct{}),
		release: make(chan struct{}),
		models:  []Model{{ID: "old-model"}},
	}
	catalog, err := NewCatalog(r, discoverer)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	refreshErr := make(chan error, 1)
	go func() {
		_, err := catalog.Refresh(t.Context(), "custom")
		refreshErr <- err
	}()
	<-discoverer.started
	p.BaseURL = "https://new.example/v1"
	if _, err := r.Save(t.Context(), p, nil); err != nil {
		t.Fatalf("Save(new BaseURL) error = %v", err)
	}
	close(discoverer.release)
	if err := <-refreshErr; !errors.Is(err, ErrProviderChanged) {
		t.Fatalf("Refresh() error = %v, want ErrProviderChanged", err)
	}
	snapshot, err := r.ModelSnapshot(t.Context(), "custom")
	if err != nil {
		t.Fatalf("ModelSnapshot() error = %v", err)
	}
	if !snapshot.RefreshedAt.IsZero() {
		t.Fatalf("stale snapshot was persisted: %+v", snapshot)
	}
}

type fakeDiscoverer struct {
	models []Model
	err    error
}

type blockingDiscoverer struct {
	started chan struct{}
	release chan struct{}
	models  []Model
}

func (d *blockingDiscoverer) Discover(ctx context.Context, _ Provider) ([]Model, error) {
	close(d.started)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-d.release:
		return cloneModels(d.models), nil
	}
}

func (f fakeDiscoverer) Discover(ctx context.Context, _ Provider) ([]Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return cloneModels(f.models), f.err
}

func findModel(models []Model, id string) (Model, bool) {
	for _, model := range models {
		if model.ID == id {
			return model, true
		}
	}
	return Model{}, false
}
