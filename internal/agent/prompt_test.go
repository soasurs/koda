package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/soasurs/adk/agent/llmagent"

	"github.com/soasurs/koda/internal/permission"
)

func TestStaticInstructionComposition(t *testing.T) {
	common, err := embeddedPrompt("prompts/common.md")
	if err != nil {
		t.Fatal(err)
	}
	build, err := embeddedPrompt("prompts/build.md")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := embeddedPrompt("prompts/plan.md")
	if err != nil {
		t.Fatal(err)
	}
	buildInstruction, err := staticInstruction(ModeBuild)
	if err != nil {
		t.Fatal(err)
	}
	planInstruction, err := staticInstruction(ModePlan)
	if err != nil {
		t.Fatal(err)
	}
	if buildInstruction != common+"\n\n"+build || planInstruction != common+"\n\n"+plan {
		t.Fatal("static instruction order or separators changed")
	}
	if strings.Count(buildInstruction, common) != 1 || strings.Count(planInstruction, common) != 1 {
		t.Fatal("common prompt must appear exactly once")
	}
	if strings.Contains(buildInstruction, plan) || strings.Contains(planInstruction, build) {
		t.Fatal("mode prompts leaked into each other")
	}
}

func TestCompactionPromptsEmbedVersionedSchema(t *testing.T) {
	prompt, err := embeddedPrompt("prompts/compaction.md")
	if err != nil {
		t.Fatal(err)
	}
	verify, err := embeddedPrompt("prompts/compaction_verify.md")
	if err != nil {
		t.Fatal(err)
	}
	repair, err := embeddedPrompt("prompts/compaction_repair.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"segment_summary"`, `"state_snapshot"`, `"schema_version"`,
		`"objective"`, `"user_requirements"`, `"constraints"`,
		`"confirmed_facts"`, `"hypotheses"`, `"completed_work"`,
		`"current_progress"`, `"pending_work"`, `"relevant_files"`,
		`"relevant_symbols"`, `"commands_and_results"`,
		`"errors_and_failures"`, `"open_questions"`, `"next_steps"`,
	} {
		if !strings.Contains(prompt, field) {
			t.Fatalf("compaction prompt does not contain %s", field)
		}
	}
	if strings.Count(prompt, `"schema_version": 1`) != 2 ||
		!strings.Contains(prompt, "failed or was interrupted") ||
		!strings.Contains(prompt, "safe prefix") ||
		!strings.Contains(verify, "version 1") || !strings.Contains(repair, "version 1") {
		t.Fatalf("compaction schema version missing: prompt %q, verify %q, repair %q", prompt, verify, repair)
	}
}

func TestRuntimeInstructionPermissions(t *testing.T) {
	workdir := t.TempDir()
	tests := []struct {
		mode  Mode
		file  permission.FileAccess
		shell permission.ShellAccess
		want  []string
	}{
		{ModeBuild, permission.FileAccessWorkspaceRead, permission.ShellAccessApprovalRequired, []string{"Workspace reads are automatic", "Every shell command requires approval"}},
		{ModeBuild, permission.FileAccessWorkspaceWrite, permission.ShellAccessUnrestricted, []string{"Workspace reads and writes are automatic", "effective access to the full filesystem"}},
		{ModeBuild, permission.FileAccessUnrestricted, permission.ShellAccessApprovalRequired, []string{"All filesystem reads and writes are automatic", "Every shell command requires approval"}},
		{ModePlan, permission.FileAccessWorkspaceRead, permission.ShellAccessApprovalRequired, []string{"do not add tools", "allowlisted read-only Git"}},
		{ModePlan, permission.FileAccessWorkspaceWrite, permission.ShellAccessUnrestricted, []string{"do not add tools", "even when session shell access is unrestricted"}},
		{ModePlan, permission.FileAccessUnrestricted, permission.ShellAccessUnrestricted, []string{"filesystem writes", "unavailable"}},
	}
	for _, test := range tests {
		name := string(test.mode) + "/" + string(test.file) + "/" + string(test.shell)
		t.Run(name, func(t *testing.T) {
			instruction, err := runtimeInstruction(test.mode, RunEnvironment{Workdir: workdir, FileAccess: test.file, ShellAccess: test.shell})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range test.want {
				if !strings.Contains(instruction, want) {
					t.Fatalf("instruction = %q, want %q", instruction, want)
				}
			}
		})
	}
}

func TestRuntimeInstructionDescribesWorkingDirectoryBehavior(t *testing.T) {
	instruction, err := runtimeInstruction(ModeBuild, RunEnvironment{
		Workdir:     t.TempDir(),
		FileAccess:  permission.FileAccessWorkspaceRead,
		ShellAccess: permission.ShellAccessApprovalRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Relative tool paths are resolved against the working directory",
		"`run_shell`",
		"starts there by default",
		"use relative paths",
		"Do not prepend the working directory or run an unnecessary `cd`",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("instruction = %q, want %q", instruction, want)
		}
	}
}

func TestInstructionProviderValidatesEnvironmentAndEscapesWorkdir(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "work`space\n# injected")
	if err := os.Mkdir(workdir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, provider, _, err := instructionConfiguration(ModeBuild, workdir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider(t.Context(), llmagent.InstructionInput{}); err == nil {
		t.Fatal("provider without environment error = nil")
	}
	for _, environment := range []RunEnvironment{
		{Workdir: "", FileAccess: permission.FileAccessWorkspaceRead, ShellAccess: permission.ShellAccessApprovalRequired},
		{Workdir: workdir, FileAccess: "invalid", ShellAccess: permission.ShellAccessApprovalRequired},
		{Workdir: workdir, FileAccess: permission.FileAccessWorkspaceRead, ShellAccess: "invalid"},
	} {
		ctx := WithRunEnvironment(t.Context(), environment)
		if _, err := provider(ctx, llmagent.InstructionInput{}); err == nil {
			t.Fatalf("provider(%+v) error = nil", environment)
		}
	}
	ctx := WithRunEnvironment(t.Context(), RunEnvironment{Workdir: workdir, FileAccess: permission.FileAccessWorkspaceRead, ShellAccess: permission.ShellAccessApprovalRequired})
	instruction, err := provider(ctx, llmagent.InstructionInput{})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := normalizeWorkdir(workdir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instruction, "Working directory: "+strconv.Quote(resolved)) || strings.Contains(instruction, "\n# injected\n") {
		t.Fatalf("instruction = %q, want escaped workdir", instruction)
	}
}

func TestInstructionConfigurationCapturesWorkspaceSnapshot(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "workspace")
	if err := os.Mkdir(workdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("parent"), 0o600); err != nil {
		t.Fatal(err)
	}
	childPath := filepath.Join(workdir, "AGENTS.md")
	if err := os.WriteFile(childPath, []byte("child v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, provider, firstHash, err := instructionConfiguration(ModePlan, workdir, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte("child v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := WithRunEnvironment(t.Context(), RunEnvironment{Workdir: workdir, FileAccess: permission.FileAccessWorkspaceRead, ShellAccess: permission.ShellAccessApprovalRequired})
	captured, err := provider(ctx, llmagent.InstructionInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(captured, "parent") || !strings.Contains(captured, "child v1") || strings.Contains(captured, "child v2") || strings.Index(captured, "parent") > strings.Index(captured, "child v1") {
		t.Fatalf("captured instructions = %q", captured)
	}
	_, _, secondHash, err := instructionConfiguration(ModePlan, workdir, "")
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("workspace instruction change did not change cache fingerprint")
	}
}

func TestInstructionConfigurationIncludesProcessSkillCatalog(t *testing.T) {
	workdir := t.TempDir()
	_, provider, withSkillsHash, err := instructionConfiguration(ModeBuild, workdir, "- review-go: Review Go code.")
	if err != nil {
		t.Fatal(err)
	}
	_, _, withoutSkillsHash, err := instructionConfiguration(ModeBuild, workdir, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRunEnvironment(t.Context(), RunEnvironment{
		Workdir: workdir, FileAccess: permission.FileAccessWorkspaceRead, ShellAccess: permission.ShellAccessApprovalRequired,
	})
	instruction, err := provider(ctx, llmagent.InstructionInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instruction, "# Available skills\n\n- review-go: Review Go code.") {
		t.Fatalf("instruction = %q", instruction)
	}
	if withSkillsHash == withoutSkillsHash {
		t.Fatal("skill catalog did not change instruction fingerprint")
	}
}
