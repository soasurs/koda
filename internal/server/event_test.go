package server

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	sessionevent "github.com/soasurs/adk/session/event"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	"google.golang.org/protobuf/proto"
)

func TestEventHistoryHandlers(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: proto.String(t.TempDir()), ProviderId: proto.String("openai-responses"), ModelId: proto.String("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	adkSession, err := handler.store.EnsureADKSession(t.Context(), created.GetSession().GetId())
	if err != nil {
		t.Fatalf("EnsureADKSession() error = %v", err)
	}
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	for _, event := range []*sessionevent.Event{
		{
			EventID: 1, TurnID: "turn-1", Author: "user", Role: string(model.RoleUser),
			Parts: sessionevent.Parts{
				{Type: model.ContentPartTypeText, Text: "review this"},
				{Type: model.ContentPartTypeImageURL, ImageURL: "https://example.com/diagram.png", ImageDetail: model.ImageDetailHigh},
			},
			CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
		},
		{
			EventID: 2, TurnID: "turn-1", Author: "assistant", Role: string(model.RoleAssistant), Content: "looks good",
			FinishReason: string(model.FinishReasonStop),
			CreatedAt:    now.Add(time.Millisecond).UnixMilli(), UpdatedAt: now.Add(time.Millisecond).UnixMilli(),
		},
	} {
		if err := adkSession.CreateEvent(t.Context(), event); err != nil {
			t.Fatalf("CreateEvent(%d) error = %v", event.EventID, err)
		}
	}

	sessionID := created.GetSession().GetId()
	listed, err := client.ListEvents(t.Context(), v1.ListEventsRequest_builder{SessionId: proto.String(sessionID), Limit: proto.Int32(1)}.Build())
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if listed.GetTotal() != 2 || len(listed.GetEvents()) != 1 || listed.GetEvents()[0].GetId() != "1" ||
		listed.GetEvents()[0].GetMessage().GetRole() != v1.Role_ROLE_USER || len(listed.GetEvents()[0].GetMessage().GetParts()) != 2 ||
		listed.GetEvents()[0].GetMessage().GetParts()[1].GetImage().GetUrl() != "https://example.com/diagram.png" {
		t.Fatalf("ListEvents() = %+v", listed)
	}

	undone, err := client.UndoLastMessage(t.Context(), v1.UndoLastMessageRequest_builder{SessionId: proto.String(sessionID)}.Build())
	if err != nil {
		t.Fatalf("UndoLastMessage() error = %v", err)
	}
	if undone.GetTurnId() != "turn-1" || undone.GetDeletedEventCount() != 2 || undone.GetInput() == nil || len(undone.GetInput().GetParts()) != 2 ||
		undone.GetInput().GetParts()[0].GetText() != "review this" || undone.GetInput().GetParts()[1].GetImage().GetUrl() != "https://example.com/diagram.png" {
		t.Fatalf("UndoLastMessage() = %+v", undone)
	}

	listed, err = client.ListEvents(t.Context(), v1.ListEventsRequest_builder{SessionId: proto.String(sessionID)}.Build())
	if err != nil {
		t.Fatalf("ListEvents(after undo) error = %v", err)
	}
	if listed.GetTotal() != 0 || len(listed.GetEvents()) != 0 {
		t.Fatalf("ListEvents(after undo) = %+v", listed)
	}
	empty, err := client.UndoLastMessage(t.Context(), v1.UndoLastMessageRequest_builder{SessionId: proto.String(sessionID)}.Build())
	if err != nil {
		t.Fatalf("UndoLastMessage(empty) error = %v", err)
	}
	if empty.GetTurnId() != "" || empty.GetDeletedEventCount() != 0 || empty.GetInput() != nil {
		t.Fatalf("UndoLastMessage(empty) = %+v", empty)
	}
}

func TestEventHistoryHandlersValidateRequests(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: proto.String(t.TempDir()), ProviderId: proto.String("openai-responses"), ModelId: proto.String("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	sessionID := created.GetSession().GetId()
	if _, err := client.ListEvents(t.Context(), v1.ListEventsRequest_builder{SessionId: proto.String(sessionID), Limit: proto.Int32(-1)}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListEvents(negative limit) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.ListEvents(t.Context(), v1.ListEventsRequest_builder{SessionId: proto.String(sessionID), Offset: proto.Int64(-1)}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListEvents(negative offset) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.UndoLastMessage(t.Context(), v1.UndoLastMessageRequest_builder{SessionId: proto.String("missing")}.Build()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("UndoLastMessage(missing) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}
