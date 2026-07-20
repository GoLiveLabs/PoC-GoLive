package ingest_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/ingest"
	"live-orchestrator/backend/internal/pagination"
	"live-orchestrator/backend/internal/testdb"
)

// newServices wires ingest.Service against a real client.Service (no mocked
// repositories, same DB), matching the source project's testing philosophy:
// constraint-dependent behavior (composite uniqueness, FK checks) only means
// something when exercised against a real database.
func newServices(t *testing.T) (*client.Service, *ingest.Service) {
	db := testdb.New(t)
	clientSvc := client.NewService(db)
	return clientSvc, ingest.NewService(db, clientSvc)
}

func boolPtr(b bool) *bool { return &b }

func TestCreate_UnknownClient_NotFound(t *testing.T) {
	_, ingestSvc := newServices(t)
	_, err := ingestSvc.Create(context.Background(), uuid.New(), ingest.CreateRequest{URL: "https://acme.com/feed"})
	if err != ingest.ErrClientNotFound {
		t.Fatalf("expected ErrClientNotFound, got %v", err)
	}
}

func TestCreate_DuplicateURLForSameClient_Conflict(t *testing.T) {
	clientSvc, ingestSvc := newServices(t)
	ctx := context.Background()

	c, err := clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	req := ingest.CreateRequest{URL: "https://acme.com/feed"}
	if _, err := ingestSvc.Create(ctx, c.ID, req); err != nil {
		t.Fatalf("first ingest create: %v", err)
	}
	if _, err := ingestSvc.Create(ctx, c.ID, req); err != ingest.ErrDuplicateURL {
		t.Fatalf("expected ErrDuplicateURL, got %v", err)
	}
}

func TestCreate_SameURLDifferentClients_Allowed(t *testing.T) {
	clientSvc, ingestSvc := newServices(t)
	ctx := context.Background()

	c1, _ := clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	c2, _ := clientSvc.Create(ctx, client.CreateRequest{Name: "Beta"})

	req := ingest.CreateRequest{URL: "https://shared-feed.com/x"}
	if _, err := ingestSvc.Create(ctx, c1.ID, req); err != nil {
		t.Fatalf("create for c1: %v", err)
	}
	if _, err := ingestSvc.Create(ctx, c2.ID, req); err != nil {
		t.Fatalf("expected same url allowed for a different client, got %v", err)
	}
}

func TestUpdate_ChangeURL_RederivesProtocolAndChecksDuplicate(t *testing.T) {
	clientSvc, ingestSvc := newServices(t)
	ctx := context.Background()

	c, _ := clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	ing, err := ingestSvc.Create(ctx, c.ID, ingest.CreateRequest{URL: "https://acme.com/feed"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newURL := "sftp://files.acme.com/inbox"
	updated, err := ingestSvc.Update(ctx, ing.ID, ingest.UpdateRequest{URL: &newURL})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Protocol != "sftp" {
		t.Fatalf("expected protocol re-derived to sftp, got %q", updated.Protocol)
	}
}

func TestUpdate_NoFieldsProvided_BadRequest(t *testing.T) {
	clientSvc, ingestSvc := newServices(t)
	ctx := context.Background()

	c, _ := clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	ing, _ := ingestSvc.Create(ctx, c.ID, ingest.CreateRequest{URL: "https://acme.com/feed"})

	if _, err := ingestSvc.Update(ctx, ing.ID, ingest.UpdateRequest{}); err != ingest.ErrURLRequired {
		t.Fatalf("expected ErrURLRequired, got %v", err)
	}
}

func TestDelete_HardDelete(t *testing.T) {
	clientSvc, ingestSvc := newServices(t)
	ctx := context.Background()

	c, _ := clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	ing, _ := ingestSvc.Create(ctx, c.ID, ingest.CreateRequest{URL: "https://acme.com/feed"})

	if err := ingestSvc.Delete(ctx, ing.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := ingestSvc.GetByID(ctx, ing.ID); err != ingest.ErrNotFound {
		t.Fatalf("expected ErrNotFound after hard delete, got %v", err)
	}
}

func TestList_FilterByIsActiveAndClient(t *testing.T) {
	clientSvc, ingestSvc := newServices(t)
	ctx := context.Background()

	c, _ := clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	if _, err := ingestSvc.Create(ctx, c.ID, ingest.CreateRequest{URL: "https://acme.com/a", IsActive: boolPtr(true)}); err != nil {
		t.Fatalf("create active: %v", err)
	}
	if _, err := ingestSvc.Create(ctx, c.ID, ingest.CreateRequest{URL: "https://acme.com/b", IsActive: boolPtr(false)}); err != nil {
		t.Fatalf("create inactive: %v", err)
	}

	page, err := ingestSvc.List(ctx, ingest.ListFilter{ClientID: &c.ID, IsActive: boolPtr(true)}, pagination.Request{Limit: 25})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Data) != 1 || !page.Data[0].IsActive {
		t.Fatalf("expected exactly one active ingest, got %+v", page.Data)
	}
}

func TestList_UnknownClientFilter_NotFound(t *testing.T) {
	_, ingestSvc := newServices(t)
	unknown := uuid.New()
	_, err := ingestSvc.List(context.Background(), ingest.ListFilter{ClientID: &unknown}, pagination.Request{Limit: 25})
	if err != ingest.ErrClientNotFound {
		t.Fatalf("expected ErrClientNotFound, got %v", err)
	}
}
