package server

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	sessionevent "github.com/soasurs/adk/session/event"
	v1 "github.com/soasurs/koda/gen/koda/v1"
)

func TestEventHistoryHandlers(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: t.TempDir(), ProviderId: "openai-responses", ModelId: "gpt-5.6",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	adkSession, err := handler.store.EnsureADKSession(t.Context(), created.Session.Id)
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

	listed, err := client.ListEvents(t.Context(), &v1.ListEventsRequest{SessionId: created.Session.Id, Limit: 1})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if listed.Total != 2 || len(listed.Events) != 1 || listed.Events[0].Id != "1" ||
		listed.Events[0].Message.Role != v1.Role_ROLE_USER || len(listed.Events[0].Message.Parts) != 2 ||
		listed.Events[0].Message.Parts[1].GetImage().GetUrl() != "https://example.com/diagram.png" {
		t.Fatalf("ListEvents() = %+v", listed)
	}

	undone, err := client.UndoLastMessage(t.Context(), &v1.UndoLastMessageRequest{SessionId: created.Session.Id})
	if err != nil {
		t.Fatalf("UndoLastMessage() error = %v", err)
	}
	if undone.TurnId != "turn-1" || undone.DeletedEventCount != 2 || undone.Input == nil || len(undone.Input.Parts) != 2 ||
		undone.Input.Parts[0].GetText() != "review this" || undone.Input.Parts[1].GetImage().GetUrl() != "https://example.com/diagram.png" {
		t.Fatalf("UndoLastMessage() = %+v", undone)
	}

	listed, err = client.ListEvents(t.Context(), &v1.ListEventsRequest{SessionId: created.Session.Id})
	if err != nil {
		t.Fatalf("ListEvents(after undo) error = %v", err)
	}
	if listed.Total != 0 || len(listed.Events) != 0 {
		t.Fatalf("ListEvents(after undo) = %+v", listed)
	}
	empty, err := client.UndoLastMessage(t.Context(), &v1.UndoLastMessageRequest{SessionId: created.Session.Id})
	if err != nil {
		t.Fatalf("UndoLastMessage(empty) error = %v", err)
	}
	if empty.TurnId != "" || empty.DeletedEventCount != 0 || empty.Input != nil {
		t.Fatalf("UndoLastMessage(empty) = %+v", empty)
	}
}

func TestEventHistoryHandlersValidateRequests(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: t.TempDir(), ProviderId: "openai-responses", ModelId: "gpt-5.6",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := client.ListEvents(t.Context(), &v1.ListEventsRequest{SessionId: created.Session.Id, Limit: -1}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListEvents(negative limit) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.ListEvents(t.Context(), &v1.ListEventsRequest{SessionId: created.Session.Id, Offset: -1}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListEvents(negative offset) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.UndoLastMessage(t.Context(), &v1.UndoLastMessageRequest{SessionId: "missing"}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("UndoLastMessage(missing) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}
