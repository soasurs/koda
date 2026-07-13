package server

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	adkskill "github.com/soasurs/adk/skill"
	v1 "github.com/soasurs/koda/gen/koda/v1"
)

// ListSkills returns summaries of the Agent Skills loaded when Koda started.
func (h *Handler) ListSkills(ctx context.Context, _ *v1.ListSkillsRequest) (*v1.ListSkillsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, skillContextError(err)
	}
	skills := h.skills.Skills()
	result := make([]*v1.SkillSummary, len(skills))
	for index, skill := range skills {
		result[index] = v1.SkillSummary_builder{
			Name:        proto.String(skill.Name),
			Description: proto.String(skill.Description),
		}.Build()
	}
	return v1.ListSkillsResponse_builder{Skills: result}.Build(), nil
}

// GetSkill returns one complete Agent Skill definition loaded when Koda started.
func (h *Handler) GetSkill(ctx context.Context, request *v1.GetSkillRequest) (*v1.GetSkillResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("get skill request must not be nil"))
	}
	if err := ctx.Err(); err != nil {
		return nil, skillContextError(err)
	}
	name := strings.TrimSpace(request.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("skill name must not be empty"))
	}
	for _, skill := range h.skills.Skills() {
		if skill.Name == name {
			return v1.GetSkillResponse_builder{Skill: skillToProto(skill)}.Build(), nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, errors.New("skill not found"))
}

func skillToProto(skill adkskill.Skill) *v1.Skill {
	return v1.Skill_builder{
		Name:          proto.String(skill.Name),
		Description:   proto.String(skill.Description),
		License:       proto.String(skill.License),
		Compatibility: proto.String(skill.Compatibility),
		Metadata:      skill.Metadata,
		AllowedTools:  skill.AllowedTools,
		Instructions:  proto.String(skill.Instructions),
		Resources:     skill.Resources,
	}.Build()
}

func skillContextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	}
	return connect.NewError(connect.CodeCanceled, err)
}
