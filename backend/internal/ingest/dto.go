package ingest

import (
	"time"

	"github.com/google/uuid"
)

// CreateRequest omits Protocol on purpose: it is derived from the URL scheme.
type CreateRequest struct {
	URL      string `json:"url"`
	IsActive *bool  `json:"isActive"`
}

func (r CreateRequest) ActiveOrDefault() bool {
	if r.IsActive == nil {
		return true
	}
	return *r.IsActive
}

// UpdateRequest applies a partial update: an absent field is left untouched.
// Changing URL re-derives Protocol automatically. Neither field supports an
// explicit "clear" (unlike client.email) so a plain optional pointer is
// enough here — no raw-JSON presence tracking needed.
type UpdateRequest struct {
	URL      *string `json:"url"`
	IsActive *bool   `json:"isActive"`
}

// Response is the wire representation of an ingest.
type Response struct {
	ID        uuid.UUID `json:"id"`
	ClientID  uuid.UUID `json:"clientId"`
	URL       string    `json:"url"`
	Protocol  string    `json:"protocol"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func ToResponse(i *Ingest) Response {
	return Response{
		ID:        i.ID,
		ClientID:  i.ClientID,
		URL:       i.URL,
		Protocol:  i.Protocol,
		IsActive:  i.IsActive,
		CreatedAt: i.CreatedAt,
		UpdatedAt: i.UpdatedAt,
	}
}

func ToResponses(is []*Ingest) []Response {
	out := make([]Response, 0, len(is))
	for _, i := range is {
		out = append(out, ToResponse(i))
	}
	return out
}
