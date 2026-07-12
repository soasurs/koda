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
	"time"
)

var (
	// ErrNotFound indicates that a provider ID is not registered.
	ErrNotFound = errors.New("provider not found")
	// ErrBuiltinProvider indicates that an operation cannot be applied to a
	// built-in provider.
	ErrBuiltinProvider = errors.New("built-in provider cannot be deleted")
	// ErrProviderChanged indicates that a long-running operation used stale
	// provider connection settings.
	ErrProviderChanged = errors.New("provider connection changed")
)

type storedProvider struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Type              Type    `json:"type"`
	APIKey            string  `json:"api_key,omitempty"`
	BaseURL           string  `json:"base_url,omitempty"`
	ModelOverrides    []Model `json:"model_overrides,omitempty"`
	DiscoveredModels  []Model `json:"discovered_models,omitempty"`
	ModelsRefreshedAt int64   `json:"models_refreshed_at,omitempty"`
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
	snapshots    map[string]ModelSnapshot
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
		snapshots:    make(map[string]ModelSnapshot),
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

// Get returns one provider with its resolved credential and cloned model
// overrides. Built-in environment variables take precedence over stored API
// keys.
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

// SetModelSnapshot replaces the last successfully discovered model list for
// id when providerRevision still matches. It does not change the provider
// connection revision.
func (r *Registry) SetModelSnapshot(
	ctx context.Context,
	id string,
	providerRevision uint64,
	snapshot ModelSnapshot,
) (ModelSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ModelSnapshot{}, err
	}
	id = strings.TrimSpace(id)
	models, err := normalizeModels(snapshot.Models)
	if err != nil {
		return ModelSnapshot{}, fmt.Errorf("provider registry: set model snapshot: %w", err)
	}
	if snapshot.RefreshedAt.IsZero() {
		return ModelSnapshot{}, fmt.Errorf("provider registry: set model snapshot: refreshed time must not be zero")
	}
	snapshot.Models = models
	snapshot.RefreshedAt = snapshot.RefreshedAt.UTC()

	r.mu.Lock()
	defer r.mu.Unlock()
	nextProviders := cloneProviderMap(r.providers)
	current, ok := r.providers[id]
	if !ok {
		builtin, ok := builtinProvider(id)
		if !ok {
			return ModelSnapshot{}, fmt.Errorf("provider registry: set model snapshot for %q: %w", id, ErrNotFound)
		}
		builtin.revision = 1
		current = builtin
		nextProviders[id] = builtin
	}
	if current.revision != providerRevision {
		return ModelSnapshot{}, fmt.Errorf("provider registry: set model snapshot for %q: %w", id, ErrProviderChanged)
	}
	nextSnapshots := cloneSnapshotMap(r.snapshots)
	nextSnapshots[id] = cloneSnapshot(snapshot)
	if err := r.persist(ctx, nextProviders, nextSnapshots); err != nil {
		return ModelSnapshot{}, err
	}
	r.providers = nextProviders
	r.snapshots = nextSnapshots
	return cloneSnapshot(snapshot), nil
}

// ModelSnapshot returns the last successfully discovered model list for id.
// It returns a zero snapshot when the provider has never been refreshed.
func (r *Registry) ModelSnapshot(ctx context.Context, id string) (ModelSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ModelSnapshot{}, err
	}
	id = strings.TrimSpace(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.providers[id]; !ok {
		if _, builtin := builtinProvider(id); !builtin {
			return ModelSnapshot{}, fmt.Errorf("provider registry: get model snapshot for %q: %w", id, ErrNotFound)
		}
	}
	return cloneSnapshot(r.snapshots[id]), nil
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
	nextSnapshots := cloneSnapshotMap(r.snapshots)
	delete(nextSnapshots, id)
	if err := r.persist(ctx, next, nextSnapshots); err != nil {
		return err
	}
	r.providers = next
	r.snapshots = nextSnapshots
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
			ID:             item.ID,
			Name:           item.Name,
			Type:           item.Type,
			BaseURL:        item.BaseURL,
			ModelOverrides: item.ModelOverrides,
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
		if p.builtin && p.BaseURL == "" && p.apiKey == "" {
			p.revision = 1
		} else {
			p.revision = r.nextRevision
			r.nextRevision++
		}
		r.providers[p.ID] = p

		discovered, err := normalizeModels(item.DiscoveredModels)
		if err != nil {
			return fmt.Errorf("provider registry: load %q model snapshot: %w", item.ID, err)
		}
		if len(discovered) > 0 || item.ModelsRefreshedAt != 0 {
			if item.ModelsRefreshedAt <= 0 {
				return fmt.Errorf("provider registry: load %q model snapshot: refreshed time must be positive", item.ID)
			}
			r.snapshots[p.ID] = ModelSnapshot{
				Models:      discovered,
				RefreshedAt: time.UnixMilli(item.ModelsRefreshedAt).UTC(),
			}
		}
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
	current, exists := r.providers[p.ID]
	key := ""
	if exists {
		key = current.apiKey
	}
	if apiKey != nil {
		key = strings.TrimSpace(*apiKey)
	}
	p.apiKey = key
	if !exists {
		if builtin, ok := builtinProvider(p.ID); ok {
			current = builtin
			current.revision = 1
			exists = true
		}
	}
	connectionChanged := !exists || current.Type != p.Type || current.BaseURL != p.BaseURL || current.apiKey != p.apiKey
	if connectionChanged {
		p.revision = r.nextRevision
	} else {
		p.revision = current.revision
		if p.revision == 0 {
			p.revision = 1
		}
	}

	next := cloneProviderMap(r.providers)
	next[p.ID] = cloneProvider(p)
	nextSnapshots := cloneSnapshotMap(r.snapshots)
	if err := r.persist(ctx, next, nextSnapshots); err != nil {
		return Provider{}, err
	}
	r.providers = next
	if connectionChanged {
		r.nextRevision++
	}
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

func (r *Registry) persist(ctx context.Context, providers map[string]Provider, snapshots map[string]ModelSnapshot) error {
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
		snapshot := snapshots[id]
		refreshedAt := int64(0)
		if !snapshot.RefreshedAt.IsZero() {
			refreshedAt = snapshot.RefreshedAt.UnixMilli()
		}
		stored.Providers = append(stored.Providers, storedProvider{
			ID:                p.ID,
			Name:              p.Name,
			Type:              p.Type,
			APIKey:            p.apiKey,
			BaseURL:           p.BaseURL,
			ModelOverrides:    cloneModels(p.ModelOverrides),
			DiscoveredModels:  cloneModels(snapshot.Models),
			ModelsRefreshedAt: refreshedAt,
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

func cloneSnapshotMap(snapshots map[string]ModelSnapshot) map[string]ModelSnapshot {
	result := make(map[string]ModelSnapshot, len(snapshots))
	for id, snapshot := range snapshots {
		result[id] = cloneSnapshot(snapshot)
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
