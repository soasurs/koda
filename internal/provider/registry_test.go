package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRegistryListsBuiltinsAndResolvesEnvironmentKeys(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-openai-key")
	r := openTestRegistry(t)

	providers, err := r.List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	wantIDs := []string{"anthropic", "openai", "openai-responses", "gemini", "deepseek"}
	if len(providers) != len(wantIDs) {
		t.Fatalf("len(List()) = %d, want %d", len(providers), len(wantIDs))
	}
	for i, wantID := range wantIDs {
		if providers[i].ID != wantID {
			t.Fatalf("List()[%d].ID = %q, want %q", i, providers[i].ID, wantID)
		}
		if !providers[i].Builtin() {
			t.Fatalf("List()[%d].Builtin() = false, want true", i)
		}
	}
	for _, index := range []int{1, 2} {
		if !providers[index].Configured() || providers[index].APIKey() != "env-openai-key" {
			t.Fatalf("OpenAI provider %q did not resolve its environment API key", providers[index].ID)
		}
	}
	if providers[0].Configured() {
		t.Fatalf("Anthropic Configured() = true, want false")
	}
	beforeRevision := providers[1].Revision()
	updated, err := r.Save(t.Context(), providers[1], nil)
	if err != nil {
		t.Fatalf("Save(openai) error = %v", err)
	}
	if updated.Revision() <= beforeRevision {
		t.Fatalf("updated revision = %d, want greater than %d", updated.Revision(), beforeRevision)
	}
}

func TestRegistrySavesAndReopensCustomProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".koda", "providers.json")
	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	key := "custom-key"
	saved, err := r.Save(t.Context(), Provider{
		ID:      "openrouter",
		Name:    "OpenRouter",
		Type:    TypeOpenAIChatCompletions,
		BaseURL: "https://openrouter.example/v1",
		Models: []Model{{
			ID:                     "example/model",
			ReasoningEfforts:       []string{"low", "high", "max", "ultra"},
			DefaultReasoningEffort: "high",
		}},
	}, &key)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.Name != "OpenRouter" || saved.Builtin() || !saved.Configured() {
		t.Fatalf("Save() = %+v, want configured custom provider", saved)
	}
	if saved.Models[0].Name != "example/model" {
		t.Fatalf("model name = %q, want defaulted ID", saved.Models[0].Name)
	}
	encoded, err := json.Marshal(saved)
	if err != nil {
		t.Fatalf("Marshal(provider) error = %v", err)
	}
	if strings.Contains(string(encoded), key) {
		t.Fatal("Provider JSON exposed its API key")
	}

	assertPermissions(t, filepath.Dir(path), 0o700)
	assertPermissions(t, path, 0o600)

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	got, err := reopened.Get(t.Context(), "openrouter")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.APIKey() != key || got.BaseURL != saved.BaseURL || len(got.Models) != 1 {
		t.Fatalf("reopened provider = %+v, want persisted provider", got)
	}

	updated := got
	updated.Name = "OpenRouter Updated"
	previousRevision := got.Revision()
	updated, err = reopened.Save(t.Context(), updated, nil)
	if err != nil {
		t.Fatalf("Save(preserve key) error = %v", err)
	}
	if updated.Revision() <= previousRevision {
		t.Fatalf("updated revision = %d, want greater than %d", updated.Revision(), previousRevision)
	}
	got, err = reopened.Get(t.Context(), "openrouter")
	if err != nil {
		t.Fatalf("Get(updated) error = %v", err)
	}
	if got.APIKey() != key {
		t.Fatalf("preserved API key = %q, want original key", got.APIKey())
	}

	empty := ""
	got, err = reopened.Save(t.Context(), got, &empty)
	if err != nil {
		t.Fatalf("Save(clear key) error = %v", err)
	}
	if got.Configured() {
		t.Fatalf("Configured() = true after clearing key")
	}
}

func TestRegistryWriteFailureDoesNotMutateMemory(t *testing.T) {
	r := openTestRegistry(t)
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("blocking file"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	r.path = filepath.Join(parent, "providers.json")

	_, err := r.Save(t.Context(), Provider{ID: "custom", Name: "Custom", Type: TypeOpenAIChatCompletions}, nil)
	if err == nil {
		t.Fatal("Save() error = nil, want persistence error")
	}
	if _, err := r.Get(t.Context(), "custom"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(custom) error = %v, want ErrNotFound", err)
	}
}

func TestOpenRejectsUnknownFields(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".koda")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path := filepath.Join(dir, "providers.json")
	if err := os.WriteFile(path, []byte(`{"providers":[],"unknown":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("Open() error = nil, want unknown-field error")
	}
}

func TestRegistryDoesNotPersistBuiltinEnvironmentKey(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "environment-only-key")
	path := filepath.Join(t.TempDir(), ".koda", "providers.json")
	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	deepseek, err := r.Get(t.Context(), "deepseek")
	if err != nil {
		t.Fatalf("Get(deepseek) error = %v", err)
	}
	deepseek.Models = []Model{{ID: "deepseek-v4-pro", ReasoningEfforts: []string{"high", "max"}}}
	if _, err := r.Save(t.Context(), deepseek, nil); err != nil {
		t.Fatalf("Save(deepseek) error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "environment-only-key") {
		t.Fatal("registry persisted an environment API key")
	}
}

func TestRegistrySetModelsPersistsAndReturnsClones(t *testing.T) {
	r := openTestRegistry(t)
	key := "key"
	if _, err := r.Save(t.Context(), Provider{
		ID:   "custom",
		Name: "Custom",
		Type: TypeOpenAIChatCompletions,
	}, &key); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	models, err := r.SetModels(t.Context(), "custom", []Model{{
		ID:                     "custom-model",
		ReasoningEfforts:       []string{"high", "ultra"},
		DefaultReasoningEffort: "ultra",
	}})
	if err != nil {
		t.Fatalf("SetModels() error = %v", err)
	}
	models[0].ReasoningEfforts[0] = "mutated"

	got, err := r.Get(t.Context(), "custom")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Models[0].ReasoningEfforts[0] != "high" {
		t.Fatalf("stored reasoning efforts were mutated: %v", got.Models[0].ReasoningEfforts)
	}
	got.Models[0].ReasoningEfforts[0] = "mutated-again"
	gotAgain, err := r.Get(t.Context(), "custom")
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if gotAgain.Models[0].ReasoningEfforts[0] != "high" {
		t.Fatalf("Get() returned registry-owned model data")
	}
}

func TestRegistryDelete(t *testing.T) {
	r := openTestRegistry(t)
	if _, err := r.Save(t.Context(), Provider{ID: "custom", Name: "Custom", Type: TypeGemini}, nil); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := r.Delete(t.Context(), "custom"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := r.Get(t.Context(), "custom"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(deleted) error = %v, want ErrNotFound", err)
	}
	if err := r.Delete(t.Context(), "openai"); !errors.Is(err, ErrBuiltinProvider) {
		t.Fatalf("Delete(openai) error = %v, want ErrBuiltinProvider", err)
	}
}

func TestRegistryRejectsInvalidProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
	}{
		{name: "invalid id", provider: Provider{ID: "Invalid ID", Name: "Invalid", Type: TypeOpenAIChatCompletions}},
		{name: "missing name", provider: Provider{ID: "valid", Type: TypeOpenAIChatCompletions}},
		{name: "invalid type", provider: Provider{ID: "valid", Name: "Valid", Type: Type("unknown")}},
		{name: "invalid URL", provider: Provider{ID: "valid", Name: "Valid", Type: TypeOpenAIChatCompletions, BaseURL: "not-a-url"}},
		{name: "URL credentials", provider: Provider{ID: "valid", Name: "Valid", Type: TypeOpenAIChatCompletions, BaseURL: "https://user:secret@example.com"}},
		{name: "duplicate models", provider: Provider{
			ID: "valid", Name: "Valid", Type: TypeOpenAIChatCompletions,
			Models: []Model{{ID: "same"}, {ID: "same"}},
		}},
		{name: "unsupported default effort", provider: Provider{
			ID: "valid", Name: "Valid", Type: TypeOpenAIChatCompletions,
			Models: []Model{{ID: "model", ReasoningEfforts: []string{"low"}, DefaultReasoningEffort: "high"}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := openTestRegistry(t)
			if _, err := r.Save(t.Context(), tt.provider, nil); err == nil {
				t.Fatal("Save() error = nil, want validation error")
			}
		})
	}

	r := openTestRegistry(t)
	if _, err := r.Save(t.Context(), Provider{ID: "openai", Name: "OpenAI", Type: TypeGemini}, nil); err == nil {
		t.Fatal("Save(openai with Gemini type) error = nil")
	}
}

func TestRegistryConcurrentSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".koda", "providers.json")
	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	const count = 16
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := r.Save(t.Context(), Provider{
				ID:   fmt.Sprintf("custom-%02d", i),
				Name: fmt.Sprintf("Custom %02d", i),
				Type: TypeOpenAIChatCompletions,
			}, nil)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Save() error = %v", err)
		}
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	providers, err := reopened.List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(providers), len(builtinProviders)+count; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
}

func openTestRegistry(t *testing.T) *Registry {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".koda", "providers.json")
	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return r
}

func assertPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("permissions for %q = %o, want %o", path, got, want)
	}
}
