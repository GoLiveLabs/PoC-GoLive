package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/ingest"
	"live-orchestrator/backend/internal/liveid"
	"live-orchestrator/backend/internal/pagination"
	"live-orchestrator/backend/internal/streamplatform"
)

// Hand-rolled fakes, matching the fakeOrchestrator pattern already used in
// this package: no mocking framework, just a struct with canned return
// values/errors per test.

type fakeClientService struct {
	client *client.Client
	page   pagination.Page[client.Response]
	err    error
}

func (f *fakeClientService) Create(context.Context, client.CreateRequest) (*client.Client, error) {
	return f.client, f.err
}
func (f *fakeClientService) GetByID(context.Context, uuid.UUID) (*client.Client, error) {
	return f.client, f.err
}
func (f *fakeClientService) List(context.Context, pagination.Request) (pagination.Page[client.Response], error) {
	return f.page, f.err
}
func (f *fakeClientService) Update(context.Context, uuid.UUID, client.UpdateFields) (*client.Client, error) {
	return f.client, f.err
}
func (f *fakeClientService) Delete(context.Context, uuid.UUID) error {
	return f.err
}

type fakeIngestService struct {
	ingest *ingest.Ingest
	page   pagination.Page[ingest.Response]
	err    error
}

func (f *fakeIngestService) Create(context.Context, uuid.UUID, ingest.CreateRequest) (*ingest.Ingest, error) {
	return f.ingest, f.err
}
func (f *fakeIngestService) GetByID(context.Context, uuid.UUID) (*ingest.Ingest, error) {
	return f.ingest, f.err
}
func (f *fakeIngestService) List(context.Context, ingest.ListFilter, pagination.Request) (pagination.Page[ingest.Response], error) {
	return f.page, f.err
}
func (f *fakeIngestService) Update(context.Context, uuid.UUID, ingest.UpdateRequest) (*ingest.Ingest, error) {
	return f.ingest, f.err
}
func (f *fakeIngestService) Delete(context.Context, uuid.UUID) error {
	return f.err
}

type fakePlatformService struct {
	platform *streamplatform.Platform
	page     pagination.Page[streamplatform.Response]
	err      error
}

func (f *fakePlatformService) Create(context.Context, streamplatform.CreateRequest) (*streamplatform.Platform, error) {
	return f.platform, f.err
}
func (f *fakePlatformService) GetByID(context.Context, uuid.UUID) (*streamplatform.Platform, error) {
	return f.platform, f.err
}
func (f *fakePlatformService) List(context.Context, pagination.Request) (pagination.Page[streamplatform.Response], error) {
	return f.page, f.err
}
func (f *fakePlatformService) Update(context.Context, uuid.UUID, streamplatform.UpdateRequest) (*streamplatform.Platform, error) {
	return f.platform, f.err
}
func (f *fakePlatformService) Delete(context.Context, uuid.UUID) error {
	return f.err
}

type fakeLiveIDService struct {
	liveID *liveid.ClientLiveID
	page   pagination.Page[liveid.Response]
	err    error
}

func (f *fakeLiveIDService) Create(context.Context, uuid.UUID, liveid.CreateRequest) (*liveid.ClientLiveID, error) {
	return f.liveID, f.err
}
func (f *fakeLiveIDService) GetByID(context.Context, uuid.UUID) (*liveid.ClientLiveID, error) {
	return f.liveID, f.err
}
func (f *fakeLiveIDService) List(context.Context, liveid.ListFilter, pagination.Request) (pagination.Page[liveid.Response], error) {
	return f.page, f.err
}
func (f *fakeLiveIDService) Update(context.Context, uuid.UUID, liveid.UpdateRequest) (*liveid.ClientLiveID, error) {
	return f.liveID, f.err
}
func (f *fakeLiveIDService) Delete(context.Context, uuid.UUID) error {
	return f.err
}

// newDomainTestServer builds a Server with the shared orchestrator fake
// (unused by these tests) plus the given domain fakes.
func newDomainTestServer(clients ClientService, ingests IngestService, platforms StreamPlatformService, liveIDs LiveIDService) http.Handler {
	hub := events.NewHub()
	return NewServer(&fakeOrchestrator{}, hub, testToken, clients, ingests, platforms, liveIDs, nil).Handler()
}
