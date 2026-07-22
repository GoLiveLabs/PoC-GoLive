package streamplatform

import (
	"time"

	"github.com/google/uuid"
)

// CreateRequest is the payload for adding a platform to the catalog.
type CreateRequest struct {
	Slug              string `json:"slug"`
	DisplayName       string `json:"displayName"`
	IngestURLTemplate string `json:"ingestUrlTemplate"`
}

// UpdateRequest applies a partial update: fields optional, absent means
// unchanged. Neither field supports an explicit "clear", so plain optional
// pointers are enough.
type UpdateRequest struct {
	Slug              *string `json:"slug"`
	DisplayName       *string `json:"displayName"`
	IngestURLTemplate *string `json:"ingestUrlTemplate"`
}

// Response is the wire representation of a streaming platform.
type Response struct {
	ID                uuid.UUID `json:"id"`
	Slug              string    `json:"slug"`
	DisplayName       string    `json:"displayName"`
	IngestURLTemplate string    `json:"ingestUrlTemplate"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func ToResponse(p *Platform) Response {
	return Response{
		ID:                p.ID,
		Slug:              p.Slug,
		DisplayName:       p.DisplayName,
		IngestURLTemplate: p.IngestURLTemplate,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

func ToResponses(ps []*Platform) []Response {
	out := make([]Response, 0, len(ps))
	for _, p := range ps {
		out = append(out, ToResponse(p))
	}
	return out
}
