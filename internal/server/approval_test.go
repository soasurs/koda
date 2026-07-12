package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/soasurs/koda/gen/koda/v1"
)

func TestApprovalBrokerResolvesAndCleansUp(t *testing.T) {
	broker := NewApprovalBroker()
	resolved := make(chan bool, 1)
	published := make(chan *v1.ToolApproval, 1)
	go func() {
		approved, err := broker.Await(t.Context(), &v1.ToolApproval{Id: "approval-1"}, func(approval *v1.ToolApproval) error {
			published <- approval
			return nil
		})
		if err != nil {
			t.Errorf("Await() error = %v", err)
			return
		}
		resolved <- approved
	}()

	select {
	case approval := <-published:
		if approval.Id != "approval-1" {
			t.Fatalf("published approval = %+v", approval)
		}
	case <-time.After(time.Second):
		t.Fatal("Await() did not publish")
	}
	if err := broker.Resolve("approval-1", true); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	select {
	case approved := <-resolved:
		if !approved {
			t.Fatal("Await() approved = false, want true")
		}
	case <-time.After(time.Second):
		t.Fatal("Await() did not resolve")
	}
	if err := broker.Resolve("approval-1", true); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("Resolve(resolved) error = %v, want ErrApprovalNotFound", err)
	}
}

func TestApprovalBrokerCancelsPendingApproval(t *testing.T) {
	broker := NewApprovalBroker()
	ctx, cancel := context.WithCancel(t.Context())
	published := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, err := broker.Await(ctx, &v1.ToolApproval{Id: "approval-1"}, func(*v1.ToolApproval) error {
			published <- struct{}{}
			return nil
		})
		done <- err
	}()
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("Await() did not publish")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Await() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Await() did not cancel")
	}
	if err := broker.Resolve("approval-1", true); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("Resolve(canceled) error = %v, want ErrApprovalNotFound", err)
	}
}

func TestResolveToolApprovalHandler(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	done := make(chan error, 1)
	go func() {
		_, err := handler.approvals.Await(t.Context(), &v1.ToolApproval{Id: "approval-1"}, func(*v1.ToolApproval) error { return nil })
		done <- err
	}()
	for deadline := time.Now().Add(time.Second); ; {
		handler.approvals.mu.Lock()
		_, pending := handler.approvals.pending["approval-1"]
		handler.approvals.mu.Unlock()
		if pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("approval was not registered")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := client.ResolveToolApproval(t.Context(), &v1.ResolveToolApprovalRequest{ApprovalId: "approval-1", Approved: false}); err != nil {
		t.Fatalf("ResolveToolApproval() error = %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Await() error = %v", err)
	}
	if _, err := client.ResolveToolApproval(t.Context(), &v1.ResolveToolApprovalRequest{ApprovalId: "approval-1"}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("ResolveToolApproval(expired) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}

func TestApprovalBrokerRejectsInvalidAndDuplicateRequests(t *testing.T) {
	broker := NewApprovalBroker()
	if _, err := broker.Await(nil, &v1.ToolApproval{Id: "approval-1"}, func(*v1.ToolApproval) error { return nil }); err == nil {
		t.Fatal("Await(nil context) error = nil")
	}
	if _, err := broker.Await(t.Context(), nil, func(*v1.ToolApproval) error { return nil }); err == nil {
		t.Fatal("Await(nil approval) error = nil")
	}
	if _, err := broker.Await(t.Context(), &v1.ToolApproval{}, func(*v1.ToolApproval) error { return nil }); err == nil {
		t.Fatal("Await(empty ID) error = nil")
	}
	if _, err := broker.Await(t.Context(), &v1.ToolApproval{Id: "approval-1"}, nil); err == nil {
		t.Fatal("Await(nil publish) error = nil")
	}
	if err := broker.Resolve(" ", true); err == nil {
		t.Fatal("Resolve(empty ID) error = nil")
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	published := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, err := broker.Await(ctx, &v1.ToolApproval{Id: "approval-1"}, func(*v1.ToolApproval) error {
			published <- struct{}{}
			return nil
		})
		done <- err
	}()
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("Await() did not publish")
	}
	if _, err := broker.Await(t.Context(), &v1.ToolApproval{Id: "approval-1"}, func(*v1.ToolApproval) error { return nil }); err == nil {
		t.Fatal("Await(duplicate ID) error = nil")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Await(canceled) error = %v", err)
	}

	publishErr := errors.New("stream closed")
	if _, err := broker.Await(t.Context(), &v1.ToolApproval{Id: "approval-2"}, func(*v1.ToolApproval) error { return publishErr }); !errors.Is(err, publishErr) {
		t.Fatalf("Await(publish error) error = %v, want %v", err, publishErr)
	}
	if err := broker.Resolve("approval-2", true); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("Resolve(publish failed) error = %v, want ErrApprovalNotFound", err)
	}
}
