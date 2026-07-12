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
	"google.golang.org/protobuf/proto"
)

// inputFromProto converts user-authored multimodal input into the ADK content
// supplied to a Runner. It validates only the public input contract; provider
// support is resolved later by the selected model adapter.
func inputFromProto(input *v1.Input) (model.Content, error) {
	if input == nil {
		return model.Content{}, errors.New("input must not be nil")
	}
	parts := input.GetParts()
	if len(parts) == 0 {
		return model.Content{}, errors.New("input must contain at least one part")
	}
	result := make([]model.ContentPart, len(parts))
	for index, part := range parts {
		converted, err := partFromProto(part)
		if err != nil {
			return model.Content{}, fmt.Errorf("input part %d: %w", index, err)
		}
		result[index] = converted
	}
	return model.Content{Role: model.RoleUser, Parts: result}, nil
}

func inputToProto(content model.Content) (*v1.Input, error) {
	if len(content.Parts) == 0 {
		text := strings.TrimSpace(content.Content)
		if text == "" {
			return nil, errors.New("input must contain at least one part")
		}
		part := v1.Part_builder{Text: proto.String(text)}.Build()
		return v1.Input_builder{Parts: []*v1.Part{part}}.Build(), nil
	}
	parts, err := partsToProto(content.Parts)
	if err != nil {
		return nil, err
	}
	return v1.Input_builder{Parts: parts}.Build(), nil
}

func partFromProto(part *v1.Part) (model.ContentPart, error) {
	if part == nil {
		return model.ContentPart{}, errors.New("must not be nil")
	}
	switch part.WhichContent() {
	case v1.Part_Text_case:
		return model.ContentPart{Type: model.ContentPartTypeText, Text: part.GetText()}, nil
	case v1.Part_Image_case:
		return imageFromProto(part.GetImage())
	default:
		return model.ContentPart{}, errors.New("content must be set")
	}
}

func imageFromProto(image *v1.Image) (model.ContentPart, error) {
	if image == nil {
		return model.ContentPart{}, errors.New("image must not be nil")
	}
	detail, err := imageDetailFromProto(image.GetDetail())
	if err != nil {
		return model.ContentPart{}, err
	}
	switch image.WhichSource() {
	case v1.Image_Url_case:
		imageURL := strings.TrimSpace(image.GetUrl())
		if err := validateHTTPSImageURL(imageURL); err != nil {
			return model.ContentPart{}, err
		}
		return model.ContentPart{
			Type:        model.ContentPartTypeImageURL,
			ImageURL:    imageURL,
			ImageDetail: detail,
		}, nil
	case v1.Image_Data_case:
		data := image.GetData()
		if len(data) == 0 {
			return model.ContentPart{}, errors.New("inline image data must not be empty")
		}
		mimeType, err := validateImageMIMEType(image.GetMimeType())
		if err != nil {
			return model.ContentPart{}, err
		}
		return model.ContentPart{
			Type:        model.ContentPartTypeImageBase64,
			ImageBase64: base64.StdEncoding.EncodeToString(data),
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
		return v1.Part_builder{Text: proto.String(part.Text)}.Build(), nil
	case model.ContentPartTypeImageURL:
		if err := validateHTTPSImageURL(part.ImageURL); err != nil {
			return nil, err
		}
		image := v1.Image_builder{
			Url:    proto.String(part.ImageURL),
			Detail: imageDetailToProto(part.ImageDetail).Enum(),
		}.Build()
		return v1.Part_builder{Image: image}.Build(), nil
	case model.ContentPartTypeImageBase64:
		data, err := base64.StdEncoding.DecodeString(part.ImageBase64)
		if err != nil {
			return nil, fmt.Errorf("decode inline image: %w", err)
		}
		mimeType, err := validateImageMIMEType(part.MIMEType)
		if err != nil {
			return nil, err
		}
		image := v1.Image_builder{
			Data:     data,
			MimeType: proto.String(mimeType),
			Detail:   imageDetailToProto(part.ImageDetail).Enum(),
		}.Build()
		return v1.Part_builder{Image: image}.Build(), nil
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
	converted := new(v1.Event)
	converted.SetSessionId(event.SessionID)
	converted.SetTurnId(event.TurnID)
	converted.SetAuthor(event.Author)
	converted.SetMessage(message)
	converted.SetPartial(event.Partial)
	if event.Partial {
		return converted, nil
	}
	if event.ID != 0 {
		converted.SetId(strconv.FormatInt(event.ID, 10))
	}
	converted.SetFinishReason(finishReasonToProto(event.FinishReason))
	converted.SetUsage(usageToProto(event.Usage))
	converted.SetCreatedAt(event.CreatedAt)
	converted.SetUpdatedAt(event.UpdatedAt)
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
		tc := new(v1.ToolCall)
		tc.SetId(call.ID)
		tc.SetName(call.Name)
		tc.SetArgumentsJson(string(call.Arguments))
		toolCalls[index] = tc
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
	msg := new(v1.Message)
	msg.SetRole(roleToProto(content.Role))
	msg.SetText(content.Content)
	msg.SetParts(parts)
	msg.SetReasoning(content.ReasoningContent)
	msg.SetToolCalls(toolCalls)
	msg.SetToolResponse(convertedResponse)
	return msg, nil
}

func toolResponseToProto(response *model.ToolResponse) (*v1.ToolResponse, error) {
	if response == nil {
		return nil, nil
	}
	converted := new(v1.ToolResponse)
	converted.SetToolCallId(response.ToolCallID)
	converted.SetName(response.Name)
	switch outcome := response.Outcome.(type) {
	case *tool.Result:
		if outcome == nil {
			return nil, errors.New("tool response contains a nil result")
		}
		converted.SetResult(toolResultToProto(outcome))
	case *tool.HandledError:
		if outcome == nil {
			return nil, errors.New("tool response contains a nil handled error")
		}
		errMsg := v1.ToolError_builder{
			Content:               proto.String(outcome.Content),
			StructuredContentJson: proto.String(string(outcome.StructuredContent)),
		}.Build()
		converted.SetError(errMsg)
	default:
		return nil, fmt.Errorf("tool response contains unsupported outcome %T", outcome)
	}
	return converted, nil
}

func toolResultToProto(result *tool.Result) *v1.ToolResult {
	tr := new(v1.ToolResult)
	tr.SetContent(result.Content)
	tr.SetStructuredContentJson(string(result.StructuredContent))
	tr.SetFileChanges(fileChangesFromStructuredContent(result.StructuredContent))
	return tr
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
				dl := new(v1.DiffLine)
				dl.SetKind(diffLineKindToProto(line.Kind))
				dl.SetOldLine(int32(line.OldLine))
				dl.SetNewLine(int32(line.NewLine))
				dl.SetContent(line.Content)
				lines[lineIndex] = dl
			}
			dh := new(v1.DiffHunk)
			dh.SetOldStart(int32(hunk.OldStart))
			dh.SetNewStart(int32(hunk.NewStart))
			dh.SetLines(lines)
			hunks[hunkIndex] = dh
		}
		fc := new(v1.FileChange)
		fc.SetPath(change.Path)
		fc.SetKind(fileChangeKindToProto(change.Kind))
		fc.SetHunks(hunks)
		fc.SetTruncated(change.Truncated)
		result[index] = fc
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
	converted := new(v1.TokenUsage)
	converted.SetPromptTokens(usage.PromptTokens)
	converted.SetCompletionTokens(usage.CompletionTokens)
	converted.SetTotalTokens(usage.TotalTokens)
	if usage.Details != nil {
		details := new(v1.TokenUsageDetails)
		details.SetCachedPromptTokens(usage.Details.CachedPromptTokens)
		details.SetCacheCreationPromptTokens(usage.Details.CacheCreationPromptTokens)
		details.SetCacheReadPromptTokens(usage.Details.CacheReadPromptTokens)
		details.SetReasoningTokens(usage.Details.ReasoningTokens)
		details.SetToolUsePromptTokens(usage.Details.ToolUsePromptTokens)
		details.SetAudioPromptTokens(usage.Details.AudioPromptTokens)
		details.SetAudioCompletionTokens(usage.Details.AudioCompletionTokens)
		details.SetAcceptedPredictionTokens(usage.Details.AcceptedPredictionTokens)
		details.SetRejectedPredictionTokens(usage.Details.RejectedPredictionTokens)
		converted.SetDetails(details)
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
