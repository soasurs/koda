package skills

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingDirectoryReturnsEmptyCatalog(t *testing.T) {
	catalog, err := Load(filepath.Join(t.TempDir(), "missing"), nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := catalog.Skills(); len(got) != 0 {
		t.Fatalf("Load() skills = %+v, want empty", got)
	}
}

func TestLoadDiscoversDirectChildSkills(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "review-go")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := "---\nname: review-go\ndescription: Review Go code.\n---\n\nCheck cancellation and ownership.\n"
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := Load(root, nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := catalog.Skills()
	if len(got) != 1 || got[0].Name != "review-go" || got[0].Instructions != "Check cancellation and ownership." {
		t.Fatalf("Load() skills = %+v", got)
	}
}

func TestLoadLogsAndSkipsInvalidSkill(t *testing.T) {
	root := t.TempDir()
	broken := filepath.Join(root, "broken")
	if err := os.Mkdir(broken, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "SKILL.md"), []byte("missing frontmatter"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(root, "review-go")
	if err := os.Mkdir(valid, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := "---\nname: review-go\ndescription: Review Go code.\n---\n\nCheck cancellation.\n"
	if err := os.WriteFile(filepath.Join(valid, "SKILL.md"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	catalog, err := Load(root, logger)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := catalog.Skills()
	if len(got) != 1 || got[0].Name != "review-go" {
		t.Fatalf("Load() skills = %+v, want review-go", got)
	}
	if output := logs.String(); !strings.Contains(output, "level=ERROR msg=\"load skill failed\"") || !strings.Contains(output, "skill=broken") {
		t.Fatalf("Load() log = %q", output)
	}
}
