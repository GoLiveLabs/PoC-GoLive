package liveid

import (
	"time"

	"github.com/google/uuid"
)

// CreateRequest associates a live id with a client on a given platform.
type CreateRequest struct {
	PlatformID uuid.UUID `json:"platformId"`
	LiveID     string    `json:"liveId"`
	StreamKey  string    `json:"streamKey"`
	IsActive   *bool     `json:"isActive"`
}

func (r CreateRequest) ActiveOrDefault() bool {
	if r.IsActive == nil {
		return true
	}
	return *r.IsActive
}

// UpdateRequest applies a partial update. PlatformID and ClientID cannot be
// changed here: reassigning either is a new association, not an edit of
// this one, so they simply have no place in this DTO.
type UpdateRequest struct {
	LiveID    *string `json:"liveId"`
	StreamKey *string `json:"streamKey"`
	IsActive  *bool   `json:"isActive"`
}

// Response is the wire representation of a client live id.
// StreamKey is always masked (ADR-008).
type Response struct {
	ID         uuid.UUID `json:"id"`
	ClientID   uuid.UUID `json:"clientId"`
	PlatformID uuid.UUID `json:"platformId"`
	LiveID     string    `json:"liveId"`
	StreamKey  string    `json:"streamKey"`
	IsActive   bool      `json:"isActive"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func ToResponse(l *ClientLiveID) Response {
	return Response{
		ID:         l.ID,
		ClientID:   l.ClientID,
		PlatformID: l.PlatformID,
		LiveID:     l.LiveID,
		StreamKey:  MaskStreamKey(l.StreamKey),
		IsActive:   l.IsActive,
		CreatedAt:  l.CreatedAt,
		UpdatedAt:  l.UpdatedAt,
	}
}

func ToResponses(ls []*ClientLiveID) []Response {
	out := make([]Response, 0, len(ls))
	for _, l := range ls {
		out = append(out, ToResponse(l))
	}
	return out
}
