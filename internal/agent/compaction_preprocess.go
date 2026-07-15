package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unicode/utf8"

	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/tool"
)

// DefaultCompactionToolOutputBytes bounds one tool response passed to the
// compaction model. The durable event and retained model history are unchanged.
const DefaultCompactionToolOutputBytes = 16 * 1024

// PrepareCompactionEvents clones events and reduces large or binary payloads
// before they are sent to the compaction model. Tool outputs retain their head,
// tail, original byte length, and SHA-256 digest for later identification.
func PrepareCompactionEvents(events []model.Event, maxToolOutputBytes int) ([]model.Event, error) {
	if maxToolOutputBytes < 256 {
		return nil, fmt.Errorf("agent: compaction tool output limit must be at least 256 bytes")
	}
	result := cloneEvents(events)
	for index := range result {
		content := &result[index].Content
		for partIndex, part := range content.Parts {
			if part.Type != model.ContentPartTypeImageBase64 {
				continue
			}
			digest := sha256.Sum256([]byte(part.ImageBase64))
			content.Parts[partIndex] = model.ContentPart{
				Type: model.ContentPartTypeText,
				Text: fmt.Sprintf("[base64 image omitted: mime=%s encoded_bytes=%d sha256=%s]", part.MIMEType, len(part.ImageBase64), hex.EncodeToString(digest[:])),
			}
		}
		if content.Role != model.RoleTool {
			continue
		}
		response := content.ToolResponseValue()
		text := truncateCompactionPayload(response.Text(), maxToolOutputBytes)
		var outcome tool.Outcome
		switch response.Outcome.(type) {
		case *tool.HandledError:
			outcome = tool.NewHandledError(text)
		default:
			outcome = &tool.Result{Content: text}
		}
		content.Content = text
		content.ToolResponse = &model.ToolResponse{
			ToolCallID: response.ToolCallID,
			Name:       response.Name,
			Outcome:    outcome,
		}
		content.ToolCallID = response.ToolCallID
	}
	return result, nil
}

func truncateCompactionPayload(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	digest := sha256.Sum256([]byte(value))
	omitted := len(value) - limit
	marker := compactionTruncationMarker(omitted, len(value), digest)
	for range 5 {
		available := limit - len(marker)
		head := validUTF8Prefix(value, available*2/3)
		tail := validUTF8Suffix(value, available-available*2/3)
		actualOmitted := len(value) - len(head) - len(tail)
		next := compactionTruncationMarker(actualOmitted, len(value), digest)
		if next == marker {
			return head + marker + tail
		}
		marker = next
	}
	available := limit - len(marker)
	headBytes := available * 2 / 3
	tailBytes := available - headBytes
	head := validUTF8Prefix(value, headBytes)
	tail := validUTF8Suffix(value, tailBytes)
	return head + marker + tail
}

func compactionTruncationMarker(omitted, original int, digest [sha256.Size]byte) string {
	return fmt.Sprintf("\n...[compaction truncated %d bytes; original_bytes=%d; sha256=%s]...\n", omitted, original, hex.EncodeToString(digest[:]))
}

func validUTF8Prefix(value string, size int) string {
	if size >= len(value) {
		return value
	}
	for size > 0 && !utf8.ValidString(value[:size]) {
		size--
	}
	return value[:size]
}

func validUTF8Suffix(value string, size int) string {
	if size >= len(value) {
		return value
	}
	start := len(value) - size
	for start < len(value) && !utf8.ValidString(value[start:]) {
		start++
	}
	return value[start:]
}

func cloneEvents(events []model.Event) []model.Event {
	result := make([]model.Event, len(events))
	for index, event := range events {
		result[index] = event
		result[index].Content = cloneModelContent(event.Content)
		if event.Usage != nil {
			usage := *event.Usage
			if event.Usage.Details != nil {
				details := *event.Usage.Details
				usage.Details = &details
			}
			result[index].Usage = &usage
		}
	}
	return result
}

func cloneModelContent(content model.Content) model.Content {
	result := content
	result.Parts = append([]model.ContentPart(nil), content.Parts...)
	result.ToolCalls = append([]model.ToolCall(nil), content.ToolCalls...)
	for index := range result.ToolCalls {
		result.ToolCalls[index].Arguments = append([]byte(nil), content.ToolCalls[index].Arguments...)
		result.ToolCalls[index].ThoughtSignature = append([]byte(nil), content.ToolCalls[index].ThoughtSignature...)
	}
	if content.ToolResponse != nil {
		response := *content.ToolResponse
		switch outcome := content.ToolResponse.Outcome.(type) {
		case *tool.Result:
			response.Outcome = outcome.Clone()
		case *tool.HandledError:
			response.Outcome = outcome.Clone()
		}
		result.ToolResponse = &response
	}
	return result
}
