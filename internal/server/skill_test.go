package server

import (
	"context"
	"testing"
	"testing/fstest"

	"connectrpc.com/connect"

	adkskill "github.com/soasurs/adk/skill"
	v1 "github.com/soasurs/koda/gen/koda/v1"
)

func TestListAndGetSkills(t *testing.T) {
	catalog, err := adkskill.Discover(fstest.MapFS{
		"review-go/SKILL.md":                {Data: []byte("---\nname: review-go\ndescription: Review Go code.\nlicense: MIT\ncompatibility: Go 1.26\nmetadata:\n  owner: koda\nallowed-tools: read_file search_text\n---\n\nCheck cancellation.\n")},
		"review-go/references/checklist.md": {Data: []byte("# Checklist\n")},
	}, ".")
	if err != nil {
		t.Fatalf("skill.Discover() error = %v", err)
	}
	handler := &Handler{skills: catalog}

	listed, err := handler.ListSkills(t.Context(), v1.ListSkillsRequest_builder{}.Build())
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if len(listed.GetSkills()) != 1 || listed.GetSkills()[0].GetName() != "review-go" || listed.GetSkills()[0].GetDescription() != "Review Go code." {
		t.Fatalf("ListSkills() = %+v", listed)
	}

	got, err := handler.GetSkill(t.Context(), v1.GetSkillRequest_builder{Name: new("review-go")}.Build())
	if err != nil {
		t.Fatalf("GetSkill() error = %v", err)
	}
	skill := got.GetSkill()
	if skill.GetInstructions() != "Check cancellation." || skill.GetLicense() != "MIT" || skill.GetCompatibility() != "Go 1.26" ||
		skill.GetMetadata()["owner"] != "koda" || len(skill.GetAllowedTools()) != 2 || len(skill.GetResources()) != 1 || skill.GetResources()[0] != "references/checklist.md" {
		t.Fatalf("GetSkill() = %+v", skill)
	}
}

func TestGetSkillValidatesRequest(t *testing.T) {
	handler := &Handler{}
	if _, err := handler.GetSkill(t.Context(), nil); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("GetSkill(nil) code = %v", connect.CodeOf(err))
	}
	if _, err := handler.GetSkill(t.Context(), v1.GetSkillRequest_builder{}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("GetSkill(empty) code = %v", connect.CodeOf(err))
	}
	if _, err := handler.GetSkill(t.Context(), v1.GetSkillRequest_builder{Name: new("missing")}.Build()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetSkill(missing) code = %v", connect.CodeOf(err))
	}
}

func TestListSkillsMapsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (&Handler{}).ListSkills(ctx, v1.ListSkillsRequest_builder{}.Build()); connect.CodeOf(err) != connect.CodeCanceled {
		t.Fatalf("ListSkills() code = %v", connect.CodeOf(err))
	}
}
