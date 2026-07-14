package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "koda.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nserver:\n  address: ' 127.0.0.1:8787 '\nlog:\n  level: ' WARN '\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Version != 1 || got.Server.Address != "127.0.0.1:8787" || got.Log.Level != "warn" {
		t.Fatalf("Load() = %+v", got)
	}
}

func TestLoadLogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "koda.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nlog:\n  path: ' koda.log '\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Log.Path != "koda.log" {
		t.Fatalf("Load() log.path = %q", got.Log.Path)
	}
}

func TestLoadLogOutput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "all", content: "version: 1\nlog:\n  output: ' all '\n", want: "all"},
		{name: "console", content: "version: 1\nlog:\n  output: ' console '\n", want: "console"},
		{name: "file", content: "version: 1\nlog:\n  output: ' file '\n", want: "file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "koda.yaml")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.Log.Output != test.want {
				t.Fatalf("Load() log.output = %q, want %q", got.Log.Output, test.want)
			}
		})
	}
}

func TestLoadMissingFileReturnsEmptyConfig(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil || got != (Config{}) {
		t.Fatalf("Load(missing) = %+v, %v", got, err)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing version", content: "server: {}\n"},
		{name: "unsupported version", content: "version: 2\n"},
		{name: "unknown field", content: "version: 1\nunknown: true\n"},
		{name: "invalid log level", content: "version: 1\nlog:\n  level: verbose\n"},
		{name: "invalid log output", content: "version: 1\nlog:\n  output: stdout\n"},
		{name: "multiple documents", content: "version: 1\n---\nversion: 1\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "koda.yaml")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}
