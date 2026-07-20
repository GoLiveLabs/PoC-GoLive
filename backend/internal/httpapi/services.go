package httpapi

import (
	"context"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/ingest"
	"live-orchestrator/backend/internal/liveid"
	"live-orchestrator/backend/internal/pagination"
	"live-orchestrator/backend/internal/streamplatform"
)

// ClientService is the subset of client.Service the HTTP layer depends on.
type ClientService interface {
	Create(ctx context.Context, req client.CreateRequest) (*client.Client, error)
	GetByID(ctx context.Context, id uuid.UUID) (*client.Client, error)
	List(ctx context.Context, page pagination.Request) (pagination.Page[client.Response], error)
	Update(ctx context.Context, id uuid.UUID, fields client.UpdateFields) (*client.Client, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// IngestService is the subset of ingest.Service the HTTP layer depends on.
type IngestService interface {
	Create(ctx context.Context, clientID uuid.UUID, req ingest.CreateRequest) (*ingest.Ingest, error)
	GetByID(ctx context.Context, id uuid.UUID) (*ingest.Ingest, error)
	List(ctx context.Context, filter ingest.ListFilter, page pagination.Request) (pagination.Page[ingest.Response], error)
	Update(ctx context.Context, id uuid.UUID, req ingest.UpdateRequest) (*ingest.Ingest, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// StreamPlatformService is the subset of streamplatform.Service the HTTP layer depends on.
type StreamPlatformService interface {
	Create(ctx context.Context, req streamplatform.CreateRequest) (*streamplatform.Platform, error)
	GetByID(ctx context.Context, id uuid.UUID) (*streamplatform.Platform, error)
	List(ctx context.Context, page pagination.Request) (pagination.Page[streamplatform.Response], error)
	Update(ctx context.Context, id uuid.UUID, req streamplatform.UpdateRequest) (*streamplatform.Platform, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// LiveIDService is the subset of liveid.Service the HTTP layer depends on.
type LiveIDService interface {
	Create(ctx context.Context, clientID uuid.UUID, req liveid.CreateRequest) (*liveid.ClientLiveID, error)
	GetByID(ctx context.Context, id uuid.UUID) (*liveid.ClientLiveID, error)
	List(ctx context.Context, filter liveid.ListFilter, page pagination.Request) (pagination.Page[liveid.Response], error)
	Update(ctx context.Context, id uuid.UUID, req liveid.UpdateRequest) (*liveid.ClientLiveID, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
