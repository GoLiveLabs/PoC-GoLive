package liveid_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/liveid"
	"live-orchestrator/backend/internal/streamplatform"
	"live-orchestrator/backend/internal/testdb"
)

type fixtures struct {
	clientSvc   *client.Service
	platformSvc *streamplatform.Service
	liveIDSvc   *liveid.Service
}

func newFixtures(t *testing.T) fixtures {
	db := testdb.New(t)
	clientSvc := client.NewService(db)
	platformSvc := streamplatform.NewService(db)
	return fixtures{
		clientSvc:   clientSvc,
		platformSvc: platformSvc,
		liveIDSvc:   liveid.NewService(db, clientSvc, platformSvc),
	}
}

func TestCreate_UnknownClient_NotFound(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()

	p, err := f.platformSvc.Create(ctx, streamplatform.CreateRequest{Slug: "youtube", DisplayName: "YouTube"})
	if err != nil {
		t.Fatalf("create platform: %v", err)
	}

	_, err = f.liveIDSvc.Create(ctx, uuid.New(), liveid.CreateRequest{PlatformID: p.ID, LiveID: "abc"})
	if err != liveid.ErrClientNotFound {
		t.Fatalf("expected ErrClientNotFound, got %v", err)
	}
}

func TestCreate_UnknownPlatform_NotFound(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()

	c, err := f.clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	_, err = f.liveIDSvc.Create(ctx, c.ID, liveid.CreateRequest{PlatformID: uuid.New(), LiveID: "abc"})
	if err != liveid.ErrPlatformNotFound {
		t.Fatalf("expected ErrPlatformNotFound, got %v", err)
	}
}

func TestCreate_DuplicateTuple_Conflict(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()

	c, _ := f.clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	p, _ := f.platformSvc.Create(ctx, streamplatform.CreateRequest{Slug: "youtube", DisplayName: "YouTube"})

	req := liveid.CreateRequest{PlatformID: p.ID, LiveID: "abc"}
	if _, err := f.liveIDSvc.Create(ctx, c.ID, req); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := f.liveIDSvc.Create(ctx, c.ID, req); err != liveid.ErrDuplicateLiveID {
		t.Fatalf("expected ErrDuplicateLiveID, got %v", err)
	}
}

// A client can hold several live ids on the same platform (simulcasts,
// stream history) — this is a list, not a 1:1 slot.
func TestCreate_MultipleLiveIDsSamePlatform_Allowed(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()

	c, _ := f.clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	p, _ := f.platformSvc.Create(ctx, streamplatform.CreateRequest{Slug: "youtube", DisplayName: "YouTube"})

	if _, err := f.liveIDSvc.Create(ctx, c.ID, liveid.CreateRequest{PlatformID: p.ID, LiveID: "stream-1"}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := f.liveIDSvc.Create(ctx, c.ID, liveid.CreateRequest{PlatformID: p.ID, LiveID: "stream-2"}); err != nil {
		t.Fatalf("expected a second live id on the same platform to be allowed: %v", err)
	}
}

func TestUpdate_LiveIDAndIsActive(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()

	c, _ := f.clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	p, _ := f.platformSvc.Create(ctx, streamplatform.CreateRequest{Slug: "youtube", DisplayName: "YouTube"})
	l, err := f.liveIDSvc.Create(ctx, c.ID, liveid.CreateRequest{PlatformID: p.ID, LiveID: "stream-1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newLiveID := "stream-1-renamed"
	inactive := false
	updated, err := f.liveIDSvc.Update(ctx, l.ID, liveid.UpdateRequest{LiveID: &newLiveID, IsActive: &inactive})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.LiveID != newLiveID || updated.IsActive {
		t.Fatalf("unexpected update result: %+v", updated)
	}
	// PlatformID/ClientID are not part of UpdateRequest at all — reassigning
	// either must go through a new association, never an edit of this one.
	if updated.PlatformID != p.ID || updated.ClientID != c.ID {
		t.Fatalf("platformId/clientId must remain unchanged: %+v", updated)
	}
}

func TestDelete_HardDelete(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()

	c, _ := f.clientSvc.Create(ctx, client.CreateRequest{Name: "Acme"})
	p, _ := f.platformSvc.Create(ctx, streamplatform.CreateRequest{Slug: "youtube", DisplayName: "YouTube"})
	l, err := f.liveIDSvc.Create(ctx, c.ID, liveid.CreateRequest{PlatformID: p.ID, LiveID: "stream-1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := f.liveIDSvc.Delete(ctx, l.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := f.liveIDSvc.GetByID(ctx, l.ID); err != liveid.ErrNotFound {
		t.Fatalf("expected ErrNotFound after hard delete, got %v", err)
	}
}
