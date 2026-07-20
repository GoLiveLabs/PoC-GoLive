package client

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CreateRequest is the payload for creating a client.
type CreateRequest struct {
	Name  string  `json:"name"`
	Email *string `json:"email"`
}

// UpdateFields is the parsed form of a PATCH body: "field absent" (leave
// untouched) must be distinguishable from "field sent as null" (clear it),
// which a plain `json.Unmarshal` into a `*string` field cannot do — both
// decode to a nil pointer. ParseUpdateFields resolves that ambiguity by
// checking key presence in the raw JSON object first.
type UpdateFields struct {
	Name       *string
	EmailSet   bool // true if the "email" key was present at all
	EmailValue *string
}

// ParseUpdateFields decodes a PATCH body, distinguishing an absent "email"
// key (don't touch) from an explicit "email": null (clear it).
func ParseUpdateFields(body []byte) (UpdateFields, error) {
	var raw map[string]json.RawMessage
	if len(body) > 0 {
		if err := json.Unmarshal(body, &raw); err != nil {
			return UpdateFields{}, err
		}
	}

	var fields UpdateFields
	if v, ok := raw["name"]; ok {
		var name *string
		if err := json.Unmarshal(v, &name); err != nil {
			return UpdateFields{}, err
		}
		fields.Name = name
	}
	if v, ok := raw["email"]; ok {
		fields.EmailSet = true
		var email *string
		if err := json.Unmarshal(v, &email); err != nil {
			return UpdateFields{}, err
		}
		fields.EmailValue = email
	}
	return fields, nil
}

// Response is the wire representation of a client.
type Response struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Email     *string   `json:"email,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func ToResponse(c *Client) Response {
	return Response{
		ID:        c.ID,
		Name:      c.Name,
		Email:     c.Email,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

func ToResponses(cs []*Client) []Response {
	out := make([]Response, 0, len(cs))
	for _, c := range cs {
		out = append(out, ToResponse(c))
	}
	return out
}
