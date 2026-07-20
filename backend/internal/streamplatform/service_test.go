package streamplatform_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/liveid"
	"live-orchestrator/backend/internal/pagination"
	"live-orchestrator/backend/internal/streamplatform"
	"live-orchestrator/backend/internal/testdb"
)

func newService(t *testing.T) *streamplatform.Service {
	db := testdb.New(t)
	return streamplatform.NewService(db)
}

func TestCreate_DuplicateSlug_Conflict(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, streamplatform.CreateRequest{Slug: "youtube", DisplayName: "YouTube"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// Slug comparison is case-insensitive: it's normalized to lower case.
	_, err := svc.Create(ctx, streamplatform.CreateRequest{Slug: "YouTube", DisplayName: "YouTube (dup)"})
	if err != streamplatform.ErrDuplicateSlug {
		t.Fatalf("expected ErrDuplicateSlug, got %v", err)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	svc := newService(t)
	if _, err := svc.GetByID(context.Background(), uuid.New()); err != streamplatform.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete_NotInUse_Succeeds(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	p, err := svc.Create(ctx, streamplatform.CreateRequest{Slug: "twitch", DisplayName: "Twitch"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetByID(ctx, p.ID); err != streamplatform.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDelete_ReferencedByLiveID_Conflict(t *testing.T) {
	db := testdb.New(t)
	platformSvc := streamplatform.NewService(db)
	clientSvc := client.NewService(db)
	liveIDSvc := liveid.NewService(db, clientSvc, platformSvc)
	ctx := context.Background()

	p, err := platformSvc.Create(ctx, streamplatform.CreateRequest{Slug: "kick", DisplayName: "Kick"})
	if err != nil {
		t.Fatalf("create platform: %v", err)
	}
	c, err := clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := liveIDSvc.Create(ctx, c.ID, liveid.CreateRequest{PlatformID: p.ID, LiveID: "stream-1"}); err != nil {
		t.Fatalf("create live id: %v", err)
	}

	if err := platformSvc.Delete(ctx, p.ID); err != streamplatform.ErrPlatformInUse {
		t.Fatalf("expected ErrPlatformInUse, got %v", err)
	}

	// The platform must still exist: the delete was rejected, not partially applied.
	if _, err := platformSvc.GetByID(ctx, p.ID); err != nil {
		t.Fatalf("expected platform to still exist, got %v", err)
	}
}

func TestUpdate_DuplicateSlug_Conflict(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, streamplatform.CreateRequest{Slug: "youtube", DisplayName: "YouTube"}); err != nil {
		t.Fatalf("create youtube: %v", err)
	}
	twitch, err := svc.Create(ctx, streamplatform.CreateRequest{Slug: "twitch", DisplayName: "Twitch"})
	if err != nil {
		t.Fatalf("create twitch: %v", err)
	}

	dup := "youtube"
	if _, err := svc.Update(ctx, twitch.ID, streamplatform.UpdateRequest{Slug: &dup}); err != streamplatform.ErrDuplicateSlug {
		t.Fatalf("expected ErrDuplicateSlug, got %v", err)
	}
}

func TestList_Pagination(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	for _, slug := range []string{"a", "b", "c"} {
		if _, err := svc.Create(ctx, streamplatform.CreateRequest{Slug: slug, DisplayName: slug}); err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
	}

	page, err := svc.List(ctx, pagination.Request{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Data) != 2 || !page.HasMore {
		t.Fatalf("unexpected page: %+v", page)
	}
}
