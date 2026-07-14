package server

import (
	"encoding/json"
	"testing"

	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/tool"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/tools"
)

func TestInputFromProtoPreservesMultimodalOrder(t *testing.T) {
	textPart := v1.Part_builder{Text: new("describe this")}.Build()
	imageDataPart := v1.Part_builder{Image: v1.Image_builder{
		Data:     []byte("image-bytes"),
		MimeType: new("image/png"),
		Detail:   v1.ImageDetail_IMAGE_DETAIL_HIGH.Enum(),
	}.Build()}.Build()
	imageURLPart := v1.Part_builder{Image: v1.Image_builder{
		Url:    new("https://example.com/diagram.png"),
		Detail: v1.ImageDetail_IMAGE_DETAIL_LOW.Enum(),
	}.Build()}.Build()
	input, err := inputFromProto(v1.Input_builder{Parts: []*v1.Part{textPart, imageDataPart, imageURLPart}}.Build())
	if err != nil {
		t.Fatalf("inputFromProto() error = %v", err)
	}
	if input.Role != model.RoleUser || len(input.Parts) != 3 || input.Parts[0].Text != "describe this" ||
		input.Parts[1].ImageBase64 != "aW1hZ2UtYnl0ZXM=" || input.Parts[1].MIMEType != "image/png" ||
		input.Parts[1].ImageDetail != model.ImageDetailHigh || input.Parts[2].ImageURL != "https://example.com/diagram.png" ||
		input.Parts[2].ImageDetail != model.ImageDetailLow {
		t.Fatalf("inputFromProto() = %+v", input)
	}

	roundTrip, err := inputToProto(input)
	if err != nil {
		t.Fatalf("inputToProto() error = %v", err)
	}
	if len(roundTrip.GetParts()) != 3 || roundTrip.GetParts()[0].GetText() != "describe this" ||
		string(roundTrip.GetParts()[1].GetImage().GetData()) != "image-bytes" ||
		roundTrip.GetParts()[2].GetImage().GetUrl() != "https://example.com/diagram.png" {
		t.Fatalf("inputToProto() = %+v", roundTrip)
	}
}

func TestInputFromProtoRejectsInvalidImages(t *testing.T) {
	makeImageData := func(data []byte, mimeType string) *v1.Input {
		img := v1.Image_builder{Data: data, MimeType: new(mimeType)}.Build()
		return v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Image: img}.Build()}}.Build()
	}
	makeImageURL := func(url string) *v1.Input {
		img := v1.Image_builder{Url: new(url)}.Build()
		return v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Image: img}.Build()}}.Build()
	}
	tests := []struct {
		name  string
		input *v1.Input
	}{
		{name: "empty", input: v1.Input_builder{}.Build()},
		{name: "missing part content", input: v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{}.Build()}}.Build()},
		{name: "http URL", input: makeImageURL("http://example.com/image.png")},
		{name: "URL credentials", input: makeImageURL("https://user:pass@example.com/image.png")},
		{name: "empty data", input: makeImageData(nil, "")},
		{name: "non image MIME", input: makeImageData([]byte("x"), "text/plain")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := inputFromProto(test.input); err == nil {
				t.Fatal("inputFromProto() error = nil, want validation error")
			}
		})
	}
}

func TestEventToProtoPreservesToolOutcomesAndFileChanges(t *testing.T) {
	structured := json.RawMessage(`{"path":"ignored","file_changes":[{"path":"main.go","kind":"update","hunks":[{"old_start":1,"new_start":1,"lines":[{"kind":"removed","old_line":1,"content":"old"},{"kind":"added","new_line":1,"content":"new"}]}]}]}`)
	converted, err := eventToProto(model.Event{
		ID:        42,
		SessionID: "session-1",
		TurnID:    "turn-1",
		Author:    "assistant",
		Content: model.Content{
			Role:             model.RoleTool,
			Content:          "updated main.go",
			ReasoningContent: "tool completed",
			ToolResponse: &model.ToolResponse{
				ToolCallID: "call-1",
				Name:       "edit_file",
				Outcome: &tool.Result{
					Content:           "updated main.go",
					StructuredContent: structured,
				},
			},
		},
		Usage: &model.TokenUsage{
			PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8,
			Details: &model.TokenUsageDetails{ReasoningTokens: 2},
		},
		CreatedAt: 10,
		UpdatedAt: 11,
	})
	if err != nil {
		t.Fatalf("eventToProto() error = %v", err)
	}
	if converted.GetId() != "42" || converted.GetMessage().GetRole() != v1.Role_ROLE_TOOL || converted.GetFinishReason() != v1.FinishReason_FINISH_REASON_UNSPECIFIED ||
		converted.GetUsage().GetDetails().GetReasoningTokens() != 2 || converted.GetMessage().GetToolResponse().GetResult().GetStructuredContentJson() != string(structured) {
		t.Fatalf("eventToProto() = %+v", converted)
	}
	changes := converted.GetMessage().GetToolResponse().GetResult().GetFileChanges()
	if len(changes) != 1 || changes[0].GetPath() != "main.go" || changes[0].GetKind() != v1.FileChangeKind_FILE_CHANGE_KIND_UPDATE ||
		len(changes[0].GetHunks()) != 1 || len(changes[0].GetHunks()[0].GetLines()) != 2 ||
		changes[0].GetHunks()[0].GetLines()[0].GetKind() != v1.DiffLineKind_DIFF_LINE_KIND_REMOVED ||
		changes[0].GetHunks()[0].GetLines()[1].GetKind() != v1.DiffLineKind_DIFF_LINE_KIND_ADDED {
		t.Fatalf("file changes = %+v", changes)
	}

	handled, err := toolResponseToProto(&model.ToolResponse{
		ToolCallID: "call-2",
		Outcome:    &tool.HandledError{Content: "rejected", StructuredContent: json.RawMessage(`{"reason":"user"}`)},
	})
	if err != nil {
		t.Fatalf("toolResponseToProto(handled) error = %v", err)
	}
	if handled.GetError().GetContent() != "rejected" || handled.GetError().GetStructuredContentJson() != `{"reason":"user"}` {
		t.Fatalf("toolResponseToProto(handled) = %+v", handled)
	}
}

func TestEventToProtoClearsPartialPersistenceFields(t *testing.T) {
	converted, err := eventToProto(model.Event{
		ID: 99, SessionID: "session-1", TurnID: "turn-1", Partial: true,
		Content:   model.Content{Role: model.RoleAssistant, Content: "partial"},
		CreatedAt: 10, UpdatedAt: 11,
	})
	if err != nil {
		t.Fatalf("eventToProto() error = %v", err)
	}
	if converted.GetId() != "" || converted.GetCreatedAt() != 0 || converted.GetUpdatedAt() != 0 || converted.GetFinishReason() != v1.FinishReason_FINISH_REASON_UNSPECIFIED || converted.GetUsage() != nil {
		t.Fatalf("partial event = %+v", converted)
	}
}

func TestConversionEnumMappingsAndInvalidContent(t *testing.T) {
	for _, test := range []struct {
		name   string
		detail v1.ImageDetail
		want   model.ImageDetail
	}{
		{name: "unspecified", detail: v1.ImageDetail_IMAGE_DETAIL_UNSPECIFIED, want: model.ImageDetailAuto},
		{name: "auto", detail: v1.ImageDetail_IMAGE_DETAIL_AUTO, want: model.ImageDetailAuto},
		{name: "low", detail: v1.ImageDetail_IMAGE_DETAIL_LOW, want: model.ImageDetailLow},
		{name: "high", detail: v1.ImageDetail_IMAGE_DETAIL_HIGH, want: model.ImageDetailHigh},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := imageDetailFromProto(test.detail)
			if err != nil || got != test.want || imageDetailToProto(got) != test.detail && test.detail != v1.ImageDetail_IMAGE_DETAIL_UNSPECIFIED {
				t.Fatalf("image detail conversion = %q, %v", got, err)
			}
		})
	}
	if _, err := imageDetailFromProto(v1.ImageDetail(99)); err == nil {
		t.Fatal("imageDetailFromProto(invalid) error = nil, want error")
	}
	if _, err := inputToProto(model.Content{Parts: []model.ContentPart{{Type: model.ContentPartTypeImageBase64, ImageBase64: "not base64", MIMEType: "image/png"}}}); err == nil {
		t.Fatal("inputToProto(invalid base64) error = nil, want error")
	}
	if _, err := inputToProto(model.Content{Parts: []model.ContentPart{{Type: "audio"}}}); err == nil {
		t.Fatal("inputToProto(unsupported part) error = nil, want error")
	}

	if roleToProto(model.RoleSystem) != v1.Role_ROLE_SYSTEM || roleToProto(model.RoleUser) != v1.Role_ROLE_USER ||
		roleToProto(model.RoleAssistant) != v1.Role_ROLE_ASSISTANT || roleToProto(model.RoleTool) != v1.Role_ROLE_TOOL ||
		roleToProto("unknown") != v1.Role_ROLE_UNSPECIFIED {
		t.Fatal("roleToProto() did not map every role")
	}
	if finishReasonToProto(model.FinishReasonStop) != v1.FinishReason_FINISH_REASON_STOP ||
		finishReasonToProto(model.FinishReasonLength) != v1.FinishReason_FINISH_REASON_LENGTH ||
		finishReasonToProto(model.FinishReasonToolCalls) != v1.FinishReason_FINISH_REASON_TOOL_CALLS ||
		finishReasonToProto(model.FinishReasonContentFilter) != v1.FinishReason_FINISH_REASON_CONTENT_FILTER ||
		finishReasonToProto("unknown") != v1.FinishReason_FINISH_REASON_UNSPECIFIED {
		t.Fatal("finishReasonToProto() did not map every finish reason")
	}
	if fileChangeKindToProto(tools.FileChangeCreate) != v1.FileChangeKind_FILE_CHANGE_KIND_CREATE ||
		fileChangeKindToProto(tools.FileChangeUpdate) != v1.FileChangeKind_FILE_CHANGE_KIND_UPDATE ||
		fileChangeKindToProto(tools.FileChangeDelete) != v1.FileChangeKind_FILE_CHANGE_KIND_DELETE ||
		fileChangeKindToProto("unknown") != v1.FileChangeKind_FILE_CHANGE_KIND_UNSPECIFIED {
		t.Fatal("fileChangeKindToProto() did not map every kind")
	}
	if diffLineKindToProto(tools.DiffLineContext) != v1.DiffLineKind_DIFF_LINE_KIND_CONTEXT ||
		diffLineKindToProto(tools.DiffLineAdded) != v1.DiffLineKind_DIFF_LINE_KIND_ADDED ||
		diffLineKindToProto(tools.DiffLineRemoved) != v1.DiffLineKind_DIFF_LINE_KIND_REMOVED ||
		diffLineKindToProto("unknown") != v1.DiffLineKind_DIFF_LINE_KIND_UNSPECIFIED {
		t.Fatal("diffLineKindToProto() did not map every kind")
	}
}

func TestMessageConversionPreservesToolCallsAndRejectsNilOutcomes(t *testing.T) {
	message, err := messageToProto(model.Content{
		Role:      model.RoleAssistant,
		ToolCalls: []model.ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"main.go"}`)}},
	})
	if err != nil {
		t.Fatalf("messageToProto() error = %v", err)
	}
	if len(message.GetToolCalls()) != 1 || message.GetToolCalls()[0].GetId() != "call-1" || message.GetToolCalls()[0].GetArgumentsJson() != `{"path":"main.go"}` {
		t.Fatalf("messageToProto() = %+v", message)
	}
	legacy, err := messageToProto(model.Content{
		Role: model.RoleTool, Content: "legacy result", ToolCallID: "call-legacy",
	})
	if err != nil {
		t.Fatalf("messageToProto(legacy tool) error = %v", err)
	}
	if legacy.GetToolResponse().GetToolCallId() != "call-legacy" || legacy.GetToolResponse().GetResult().GetContent() != "legacy result" {
		t.Fatalf("messageToProto(legacy tool) = %+v", legacy)
	}
	if changes := fileChangesFromStructuredContent(json.RawMessage(`not json`)); changes != nil {
		t.Fatalf("fileChangesFromStructuredContent(invalid) = %+v, want nil", changes)
	}
	var nilResult *tool.Result
	if _, err := toolResponseToProto(&model.ToolResponse{Outcome: nilResult}); err == nil {
		t.Fatal("toolResponseToProto(nil result) error = nil, want error")
	}
	var nilError *tool.HandledError
	if _, err := toolResponseToProto(&model.ToolResponse{Outcome: nilError}); err == nil {
		t.Fatal("toolResponseToProto(nil handled error) error = nil, want error")
	}
	if _, err := eventsToProto([]model.Event{{Content: model.Content{Role: model.RoleUser, Parts: []model.ContentPart{{Type: "audio"}}}}}); err == nil {
		t.Fatal("eventsToProto(invalid part) error = nil, want error")
	}
}
