package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"strconv"
	"strings"

	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/tool"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/tools"
)

// inputFromProto converts user-authored multimodal input into the ADK content
// supplied to a Runner. It validates only the public input contract; provider
// support is resolved later by the selected model adapter.
func inputFromProto(input *v1.Input) (model.Content, error) {
	if input == nil {
		return model.Content{}, errors.New("input must not be nil")
	}
	if len(input.Parts) == 0 {
		return model.Content{}, errors.New("input must contain at least one part")
	}
	parts := make([]model.ContentPart, len(input.Parts))
	for index, part := range input.Parts {
		converted, err := partFromProto(part)
		if err != nil {
			return model.Content{}, fmt.Errorf("input part %d: %w", index, err)
		}
		parts[index] = converted
	}
	return model.Content{Role: model.RoleUser, Parts: parts}, nil
}

func inputToProto(content model.Content) (*v1.Input, error) {
	if len(content.Parts) == 0 {
		return &v1.Input{Parts: []*v1.Part{{Content: &v1.Part_Text{Text: content.Content}}}}, nil
	}
	parts, err := partsToProto(content.Parts)
	if err != nil {
		return nil, err
	}
	return &v1.Input{Parts: parts}, nil
}

func partFromProto(part *v1.Part) (model.ContentPart, error) {
	if part == nil {
		return model.ContentPart{}, errors.New("must not be nil")
	}
	switch content := part.Content.(type) {
	case *v1.Part_Text:
		return model.ContentPart{Type: model.ContentPartTypeText, Text: content.Text}, nil
	case *v1.Part_Image:
		return imageFromProto(content.Image)
	default:
		return model.ContentPart{}, errors.New("content must be set")
	}
}

func imageFromProto(image *v1.Image) (model.ContentPart, error) {
	if image == nil {
		return model.ContentPart{}, errors.New("image must not be nil")
	}
	detail, err := imageDetailFromProto(image.Detail)
	if err != nil {
		return model.ContentPart{}, err
	}
	switch source := image.Source.(type) {
	case *v1.Image_Url:
		imageURL := strings.TrimSpace(source.Url)
		if err := validateHTTPSImageURL(imageURL); err != nil {
			return model.ContentPart{}, err
		}
		return model.ContentPart{
			Type:        model.ContentPartTypeImageURL,
			ImageURL:    imageURL,
			ImageDetail: detail,
		}, nil
	case *v1.Image_Data:
		if len(source.Data) == 0 {
			return model.ContentPart{}, errors.New("inline image data must not be empty")
		}
		mimeType, err := validateImageMIMEType(image.MimeType)
		if err != nil {
			return model.ContentPart{}, err
		}
		return model.ContentPart{
			Type:        model.ContentPartTypeImageBase64,
			ImageBase64: base64.StdEncoding.EncodeToString(source.Data),
			MIMEType:    mimeType,
			ImageDetail: detail,
		}, nil
	default:
		return model.ContentPart{}, errors.New("image source must be set")
	}
}

func partsToProto(parts []model.ContentPart) ([]*v1.Part, error) {
	result := make([]*v1.Part, len(parts))
	for index, part := range parts {
		converted, err := partToProto(part)
		if err != nil {
			return nil, fmt.Errorf("content part %d: %w", index, err)
		}
		result[index] = converted
	}
	return result, nil
}

func partToProto(part model.ContentPart) (*v1.Part, error) {
	switch part.Type {
	case model.ContentPartTypeText:
		return &v1.Part{Content: &v1.Part_Text{Text: part.Text}}, nil
	case model.ContentPartTypeImageURL:
		if err := validateHTTPSImageURL(part.ImageURL); err != nil {
			return nil, err
		}
		return &v1.Part{Content: &v1.Part_Image{Image: &v1.Image{
			Source: &v1.Image_Url{Url: part.ImageURL},
			Detail: imageDetailToProto(part.ImageDetail),
		}}}, nil
	case model.ContentPartTypeImageBase64:
		data, err := base64.StdEncoding.DecodeString(part.ImageBase64)
		if err != nil {
			return nil, fmt.Errorf("decode inline image: %w", err)
		}
		mimeType, err := validateImageMIMEType(part.MIMEType)
		if err != nil {
			return nil, err
		}
		return &v1.Part{Content: &v1.Part_Image{Image: &v1.Image{
			Source:   &v1.Image_Data{Data: data},
			MimeType: mimeType,
			Detail:   imageDetailToProto(part.ImageDetail),
		}}}, nil
	default:
		return nil, fmt.Errorf("unsupported content part type %q", part.Type)
	}
}

func validateHTTPSImageURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New("image URL must be an HTTPS URL without credentials")
	}
	return nil
}

func validateImageMIMEType(value string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil || !strings.HasPrefix(mediaType, "image/") {
		return "", errors.New("inline image MIME type must be an image media type")
	}
	return mediaType, nil
}

func imageDetailFromProto(detail v1.ImageDetail) (model.ImageDetail, error) {
	switch detail {
	case v1.ImageDetail_IMAGE_DETAIL_UNSPECIFIED, v1.ImageDetail_IMAGE_DETAIL_AUTO:
		return model.ImageDetailAuto, nil
	case v1.ImageDetail_IMAGE_DETAIL_LOW:
		return model.ImageDetailLow, nil
	case v1.ImageDetail_IMAGE_DETAIL_HIGH:
		return model.ImageDetailHigh, nil
	default:
		return "", fmt.Errorf("unsupported image detail %q", detail)
	}
}

func imageDetailToProto(detail model.ImageDetail) v1.ImageDetail {
	switch detail {
	case model.ImageDetailLow:
		return v1.ImageDetail_IMAGE_DETAIL_LOW
	case model.ImageDetailHigh:
		return v1.ImageDetail_IMAGE_DETAIL_HIGH
	default:
		return v1.ImageDetail_IMAGE_DETAIL_AUTO
	}
}

func eventToProto(event model.Event) (*v1.Event, error) {
	message, err := messageToProto(event.Content)
	if err != nil {
		return nil, err
	}
	converted := &v1.Event{
		SessionId: event.SessionID,
		TurnId:    event.TurnID,
		Author:    event.Author,
		Message:   message,
		Partial:   event.Partial,
	}
	if event.Partial {
		return converted, nil
	}
	if event.ID != 0 {
		converted.Id = strconv.FormatInt(event.ID, 10)
	}
	converted.FinishReason = finishReasonToProto(event.FinishReason)
	converted.Usage = usageToProto(event.Usage)
	converted.CreatedAt = event.CreatedAt
	converted.UpdatedAt = event.UpdatedAt
	return converted, nil
}

func eventsToProto(events []model.Event) ([]*v1.Event, error) {
	result := make([]*v1.Event, len(events))
	for index, event := range events {
		converted, err := eventToProto(event)
		if err != nil {
			return nil, fmt.Errorf("event %d: %w", index, err)
		}
		result[index] = converted
	}
	return result, nil
}

func messageToProto(content model.Content) (*v1.Message, error) {
	parts, err := partsToProto(content.Parts)
	if err != nil {
		return nil, err
	}
	toolCalls := make([]*v1.ToolCall, len(content.ToolCalls))
	for index, call := range content.ToolCalls {
		toolCalls[index] = &v1.ToolCall{
			Id:            call.ID,
			Name:          call.Name,
			ArgumentsJson: string(call.Arguments),
		}
	}
	toolResponse := content.ToolResponse
	if toolResponse != nil || content.Role == model.RoleTool && (content.ToolCallID != "" || content.Content != "") {
		resolved := content.ToolResponseValue()
		toolResponse = &resolved
	}
	convertedResponse, err := toolResponseToProto(toolResponse)
	if err != nil {
		return nil, err
	}
	return &v1.Message{
		Role:         roleToProto(content.Role),
		Text:         content.Content,
		Parts:        parts,
		Reasoning:    content.ReasoningContent,
		ToolCalls:    toolCalls,
		ToolResponse: convertedResponse,
	}, nil
}

func toolResponseToProto(response *model.ToolResponse) (*v1.ToolResponse, error) {
	if response == nil {
		return nil, nil
	}
	converted := &v1.ToolResponse{
		ToolCallId: response.ToolCallID,
		Name:       response.Name,
	}
	switch outcome := response.Outcome.(type) {
	case *tool.Result:
		if outcome == nil {
			return nil, errors.New("tool response contains a nil result")
		}
		converted.Outcome = &v1.ToolResponse_Result{Result: toolResultToProto(outcome)}
	case *tool.HandledError:
		if outcome == nil {
			return nil, errors.New("tool response contains a nil handled error")
		}
		converted.Outcome = &v1.ToolResponse_Error{Error: &v1.ToolError{
			Content:               outcome.Content,
			StructuredContentJson: string(outcome.StructuredContent),
		}}
	default:
		return nil, fmt.Errorf("tool response contains unsupported outcome %T", outcome)
	}
	return converted, nil
}

func toolResultToProto(result *tool.Result) *v1.ToolResult {
	return &v1.ToolResult{
		Content:               result.Content,
		StructuredContentJson: string(result.StructuredContent),
		FileChanges:           fileChangesFromStructuredContent(result.StructuredContent),
	}
}

func fileChangesFromStructuredContent(content json.RawMessage) []*v1.FileChange {
	if len(content) == 0 {
		return nil
	}
	var envelope struct {
		FileChanges []tools.FileChange `json:"file_changes"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		return nil
	}
	return fileChangesToProto(envelope.FileChanges)
}

func fileChangesToProto(changes []tools.FileChange) []*v1.FileChange {
	if len(changes) == 0 {
		return nil
	}
	result := make([]*v1.FileChange, len(changes))
	for index, change := range changes {
		hunks := make([]*v1.DiffHunk, len(change.Hunks))
		for hunkIndex, hunk := range change.Hunks {
			lines := make([]*v1.DiffLine, len(hunk.Lines))
			for lineIndex, line := range hunk.Lines {
				lines[lineIndex] = &v1.DiffLine{
					Kind:    diffLineKindToProto(line.Kind),
					OldLine: int32(line.OldLine),
					NewLine: int32(line.NewLine),
					Content: line.Content,
				}
			}
			hunks[hunkIndex] = &v1.DiffHunk{
				OldStart: int32(hunk.OldStart),
				NewStart: int32(hunk.NewStart),
				Lines:    lines,
			}
		}
		result[index] = &v1.FileChange{
			Path:      change.Path,
			Kind:      fileChangeKindToProto(change.Kind),
			Hunks:     hunks,
			Truncated: change.Truncated,
		}
	}
	return result
}

func roleToProto(role model.Role) v1.Role {
	switch role {
	case model.RoleSystem:
		return v1.Role_ROLE_SYSTEM
	case model.RoleUser:
		return v1.Role_ROLE_USER
	case model.RoleAssistant:
		return v1.Role_ROLE_ASSISTANT
	case model.RoleTool:
		return v1.Role_ROLE_TOOL
	default:
		return v1.Role_ROLE_UNSPECIFIED
	}
}

func finishReasonToProto(reason model.FinishReason) v1.FinishReason {
	switch reason {
	case model.FinishReasonStop:
		return v1.FinishReason_FINISH_REASON_STOP
	case model.FinishReasonLength:
		return v1.FinishReason_FINISH_REASON_LENGTH
	case model.FinishReasonToolCalls:
		return v1.FinishReason_FINISH_REASON_TOOL_CALLS
	case model.FinishReasonContentFilter:
		return v1.FinishReason_FINISH_REASON_CONTENT_FILTER
	default:
		return v1.FinishReason_FINISH_REASON_UNSPECIFIED
	}
}

func usageToProto(usage *model.TokenUsage) *v1.TokenUsage {
	if usage == nil {
		return nil
	}
	converted := &v1.TokenUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
	if usage.Details != nil {
		converted.Details = &v1.TokenUsageDetails{
			CachedPromptTokens:        usage.Details.CachedPromptTokens,
			CacheCreationPromptTokens: usage.Details.CacheCreationPromptTokens,
			CacheReadPromptTokens:     usage.Details.CacheReadPromptTokens,
			ReasoningTokens:           usage.Details.ReasoningTokens,
			ToolUsePromptTokens:       usage.Details.ToolUsePromptTokens,
			AudioPromptTokens:         usage.Details.AudioPromptTokens,
			AudioCompletionTokens:     usage.Details.AudioCompletionTokens,
			AcceptedPredictionTokens:  usage.Details.AcceptedPredictionTokens,
			RejectedPredictionTokens:  usage.Details.RejectedPredictionTokens,
		}
	}
	return converted
}

func fileChangeKindToProto(kind tools.FileChangeKind) v1.FileChangeKind {
	switch kind {
	case tools.FileChangeCreate:
		return v1.FileChangeKind_FILE_CHANGE_KIND_CREATE
	case tools.FileChangeUpdate:
		return v1.FileChangeKind_FILE_CHANGE_KIND_UPDATE
	case tools.FileChangeDelete:
		return v1.FileChangeKind_FILE_CHANGE_KIND_DELETE
	default:
		return v1.FileChangeKind_FILE_CHANGE_KIND_UNSPECIFIED
	}
}

func diffLineKindToProto(kind tools.DiffLineKind) v1.DiffLineKind {
	switch kind {
	case tools.DiffLineContext:
		return v1.DiffLineKind_DIFF_LINE_KIND_CONTEXT
	case tools.DiffLineAdded:
		return v1.DiffLineKind_DIFF_LINE_KIND_ADDED
	case tools.DiffLineRemoved:
		return v1.DiffLineKind_DIFF_LINE_KIND_REMOVED
	default:
		return v1.DiffLineKind_DIFF_LINE_KIND_UNSPECIFIED
	}
}
