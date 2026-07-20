package client_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/pagination"
	"live-orchestrator/backend/internal/testdb"
)

func newService(t *testing.T) *client.Service {
	db := testdb.New(t)
	return client.NewService(db)
}

func strPtr(s string) *string { return &s }

func TestCreate_DuplicateName_Conflict(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, client.CreateRequest{Name: "Acme Corp"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.Create(ctx, client.CreateRequest{Name: "Acme Corp"})
	if err != client.ErrDuplicateName {
		t.Fatalf("expected ErrDuplicateName, got %v", err)
	}
}

func TestCreate_BlankName_Invalid(t *testing.T) {
	svc := newService(t)
	_, err := svc.Create(context.Background(), client.CreateRequest{Name: "   "})
	if err != client.ErrInvalidName {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	svc := newService(t)
	_, err := svc.GetByID(context.Background(), uuid.Nil)
	if err != client.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdate_PointerSemantics_AbsentVsNullEmail(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	c, err := svc.Create(ctx, client.CreateRequest{Name: "Acme", Email: strPtr("a@acme.com")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Absent "email" key: leaves email untouched, only updates name.
	fields, err := client.ParseUpdateFields([]byte(`{"name":"Acme Ltd"}`))
	if err != nil {
		t.Fatalf("ParseUpdateFields: %v", err)
	}
	updated, err := svc.Update(ctx, c.ID, fields)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Acme Ltd" {
		t.Fatalf("expected name updated, got %q", updated.Name)
	}
	if updated.Email == nil || *updated.Email != "a@acme.com" {
		t.Fatalf("expected email untouched, got %v", updated.Email)
	}

	// Explicit "email": null clears it.
	fields, err = client.ParseUpdateFields([]byte(`{"email":null}`))
	if err != nil {
		t.Fatalf("ParseUpdateFields: %v", err)
	}
	updated, err = svc.Update(ctx, c.ID, fields)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Email != nil {
		t.Fatalf("expected email cleared, got %v", *updated.Email)
	}
}

func TestUpdate_DuplicateName_Conflict(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, client.CreateRequest{Name: "Acme"}); err != nil {
		t.Fatalf("create acme: %v", err)
	}
	other, err := svc.Create(ctx, client.CreateRequest{Name: "Beta"})
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	fields, _ := client.ParseUpdateFields([]byte(`{"name":"Acme"}`))
	_, err = svc.Update(ctx, other.ID, fields)
	if err != client.ErrDuplicateName {
		t.Fatalf("expected ErrDuplicateName, got %v", err)
	}
}

func TestDelete_SoftDelete_NotVisibleAfterwards(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	c, err := svc.Create(ctx, client.CreateRequest{Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetByID(ctx, c.ID); err != client.ErrNotFound {
		t.Fatalf("expected ErrNotFound after soft delete, got %v", err)
	}

	// A second delete of an already-deleted (or unknown) client is also 404.
	if err := svc.Delete(ctx, c.ID); err != client.ErrNotFound {
		t.Fatalf("expected ErrNotFound on repeat delete, got %v", err)
	}
}

func TestList_Pagination_OrderAndCursor(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	names := []string{"C1", "C2", "C3"}
	for _, n := range names {
		if _, err := svc.Create(ctx, client.CreateRequest{Name: n}); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}

	page, err := svc.List(ctx, pagination.Request{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Data) != 2 || !page.HasMore || page.NextCursor == nil {
		t.Fatalf("unexpected first page: %+v", page)
	}

	cursor, err := pagination.DecodeCursor(*page.NextCursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	page2, err := svc.List(ctx, pagination.Request{Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(page2.Data) != 1 || page2.HasMore {
		t.Fatalf("unexpected second page: %+v", page2)
	}
}
