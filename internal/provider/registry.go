package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

var (
	// ErrNotFound indicates that a provider ID is not registered.
	ErrNotFound = errors.New("provider not found")
	// ErrBuiltinProvider indicates that an operation cannot be applied to a
	// built-in provider.
	ErrBuiltinProvider = errors.New("built-in provider cannot be deleted")
)

type storedProvider struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Type    Type    `json:"type"`
	APIKey  string  `json:"api_key,omitempty"`
	BaseURL string  `json:"base_url,omitempty"`
	Models  []Model `json:"models,omitempty"`
}

type storedRegistry struct {
	Providers []storedProvider `json:"providers"`
}

// Registry stores built-in overrides and user-defined providers in one JSON
// file. Its methods are safe for concurrent use.
type Registry struct {
	mu           sync.RWMutex
	path         string
	providers    map[string]Provider
	nextRevision uint64
}

// DefaultPath returns the default provider registry path.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("provider registry: find home directory: %w", err)
	}
	return filepath.Join(home, ".koda", "providers.json"), nil
}

// OpenDefault opens the provider registry at DefaultPath.
func OpenDefault() (*Registry, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

// Open loads a provider registry from path. A missing file represents an empty
// set of user definitions and is created on the first write.
func Open(path string) (*Registry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("provider registry: path must not be empty")
	}
	r := &Registry{
		path:         path,
		providers:    make(map[string]Provider),
		nextRevision: 2,
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// List returns built-in providers first, followed by user-defined providers
// ordered by ID.
func (r *Registry) List(ctx context.Context) ([]Provider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Provider, 0, len(builtinProviders)+len(r.providers))
	for _, builtin := range builtinProviders {
		result = append(result, r.resolveLocked(builtin.ID))
	}
	customIDs := make([]string, 0, len(r.providers))
	for id, p := range r.providers {
		if !p.builtin {
			customIDs = append(customIDs, id)
		}
	}
	slices.Sort(customIDs)
	for _, id := range customIDs {
		result = append(result, r.resolveLocked(id))
	}
	return result, nil
}

// Get returns one provider with its resolved credential and a cloned model
// list. Built-in environment variables take precedence over stored API keys.
func (r *Registry) Get(ctx context.Context, id string) (Provider, error) {
	if err := ctx.Err(); err != nil {
		return Provider{}, err
	}
	id = strings.TrimSpace(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.providers[id]; !ok {
		if _, builtin := builtinProvider(id); !builtin {
			return Provider{}, fmt.Errorf("provider registry: get %q: %w", id, ErrNotFound)
		}
	}
	return r.resolveLocked(id), nil
}

// Save creates or replaces a provider. A nil apiKey preserves the stored key;
// a non-nil empty key clears it. Environment-provided keys are never persisted.
func (r *Registry) Save(ctx context.Context, p Provider, apiKey *string) (Provider, error) {
	if err := ctx.Err(); err != nil {
		return Provider{}, err
	}
	p, err := normalizeProvider(p)
	if err != nil {
		return Provider{}, fmt.Errorf("provider registry: save: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveLocked(ctx, p, apiKey)
}

// SetModels replaces and persists the known models for id.
func (r *Registry) SetModels(ctx context.Context, id string, models []Model) ([]Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	models, err := normalizeModels(models)
	if err != nil {
		return nil, fmt.Errorf("provider registry: set models: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.providers[id]
	if !ok {
		var builtin bool
		p, builtin = builtinProvider(id)
		if !builtin {
			return nil, fmt.Errorf("provider registry: set models for %q: %w", id, ErrNotFound)
		}
	}
	p.Models = models
	saved, err := r.saveLocked(ctx, p, nil)
	if err != nil {
		return nil, err
	}
	return cloneModels(saved.Models), nil
}

// Delete removes a user-defined provider. Built-in providers cannot be
// deleted, though their stored credentials can be cleared with Save.
func (r *Registry) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if _, builtin := builtinProvider(id); builtin {
		return fmt.Errorf("provider registry: delete %q: %w", id, ErrBuiltinProvider)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[id]; !ok {
		return fmt.Errorf("provider registry: delete %q: %w", id, ErrNotFound)
	}
	next := cloneProviderMap(r.providers)
	delete(next, id)
	if err := r.persist(ctx, next); err != nil {
		return err
	}
	r.providers = next
	return nil
}

func (r *Registry) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("provider registry: read: %w", err)
	}
	if err := os.Chmod(r.path, 0o600); err != nil {
		return fmt.Errorf("provider registry: secure file: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var stored storedRegistry
	if err := decoder.Decode(&stored); err != nil {
		return fmt.Errorf("provider registry: decode: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return fmt.Errorf("provider registry: decode: %w", err)
	}

	for _, item := range stored.Providers {
		p, err := normalizeProvider(Provider{
			ID:      item.ID,
			Name:    item.Name,
			Type:    item.Type,
			BaseURL: item.BaseURL,
			Models:  item.Models,
		})
		if err != nil {
			return fmt.Errorf("provider registry: load %q: %w", item.ID, err)
		}
		if _, exists := r.providers[p.ID]; exists {
			return fmt.Errorf("provider registry: duplicate provider id %q", p.ID)
		}
		if builtin, ok := builtinProvider(p.ID); ok {
			if p.Type != builtin.Type {
				return fmt.Errorf(
					"provider registry: built-in provider %q must use type %q",
					p.ID,
					builtin.Type,
				)
			}
			p.builtin = true
		}
		p.apiKey = strings.TrimSpace(item.APIKey)
		p.revision = r.nextRevision
		r.nextRevision++
		r.providers[p.ID] = p
	}
	return nil
}

func (r *Registry) saveLocked(ctx context.Context, p Provider, apiKey *string) (Provider, error) {
	if builtin, ok := builtinProvider(p.ID); ok {
		if p.Type != builtin.Type {
			return Provider{}, fmt.Errorf(
				"provider registry: save: built-in provider %q must use type %q",
				p.ID,
				builtin.Type,
			)
		}
		p.builtin = true
	}
	key := ""
	if current, ok := r.providers[p.ID]; ok {
		key = current.apiKey
	}
	if apiKey != nil {
		key = strings.TrimSpace(*apiKey)
	}
	p.apiKey = key
	p.revision = r.nextRevision

	next := cloneProviderMap(r.providers)
	next[p.ID] = cloneProvider(p)
	if err := r.persist(ctx, next); err != nil {
		return Provider{}, err
	}
	r.providers = next
	r.nextRevision++
	return r.resolveLocked(p.ID), nil
}

func (r *Registry) resolveLocked(id string) Provider {
	p, ok := r.providers[id]
	if !ok {
		p, _ = builtinProvider(id)
		if p.revision == 0 {
			p.revision = 1
		}
	}
	if envKey := builtinAPIKeyEnv(id); envKey != "" {
		if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
			p.apiKey = value
		}
	}
	return cloneProvider(p)
}

func (r *Registry) persist(ctx context.Context, providers map[string]Provider) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("provider registry: create directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("provider registry: secure directory: %w", err)
	}

	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	stored := storedRegistry{Providers: make([]storedProvider, 0, len(ids))}
	for _, id := range ids {
		p := providers[id]
		stored.Providers = append(stored.Providers, storedProvider{
			ID:      p.ID,
			Name:    p.Name,
			Type:    p.Type,
			APIKey:  p.apiKey,
			BaseURL: p.BaseURL,
			Models:  cloneModels(p.Models),
		})
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("provider registry: encode: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".providers-*.json")
	if err != nil {
		return fmt.Errorf("provider registry: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // The rename path no longer exists after success.
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("provider registry: secure temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("provider registry: write temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("provider registry: sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("provider registry: close temporary file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, r.path); err != nil {
		return fmt.Errorf("provider registry: replace file: %w", err)
	}
	return nil
}

func builtinAPIKeyEnv(id string) string {
	switch id {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai", "openai-responses":
		return "OPENAI_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	case "deepseek":
		return "DEEPSEEK_API_KEY"
	default:
		return ""
	}
}

func cloneProviderMap(providers map[string]Provider) map[string]Provider {
	result := make(map[string]Provider, len(providers))
	for id, p := range providers {
		result[id] = cloneProvider(p)
	}
	return result
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
