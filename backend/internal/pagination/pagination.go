// Package pagination implements opaque keyset (cursor) pagination shared by
// every listing endpoint in the client/ingest/streamplatform/liveid domains.
//
// Keyset beats OFFSET on two counts: it stays O(1) as a table grows, and it
// does not skip or repeat rows when rows are written between page fetches.
// The cursor is opaque (base64) so its shape is never part of the public
// contract and can change without breaking clients.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultLimit = 25
	MaxLimit     = 100
)

// ErrInvalidCursor is returned when a client-supplied cursor cannot be decoded.
var ErrInvalidCursor = errors.New("invalid cursor")

// ErrInvalidLimit is returned when the "limit" query param is not a positive integer.
var ErrInvalidLimit = errors.New("limit must be a positive integer")

// Cursor is a keyset pointer over (created_at DESC, id DESC).
type Cursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

// Encode produces the opaque token sent to clients as "nextCursor".
func (c Cursor) Encode() string {
	b, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor decodes a token produced by Encode. An empty string decodes to
// (nil, nil) so "no cursor" and "invalid cursor" are distinguishable.
func DecodeCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, ErrInvalidCursor
	}
	return &c, nil
}

// Request is the parsed form of the "limit"/"cursor" query params.
type Request struct {
	Limit  int
	Cursor *Cursor
}

// ParseRequest reads "limit" (default DefaultLimit, capped at MaxLimit) and
// "cursor" from the query string.
func ParseRequest(r *http.Request) (Request, error) {
	req := Request{Limit: DefaultLimit}

	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return req, ErrInvalidLimit
		}
		if n > MaxLimit {
			n = MaxLimit
		}
		req.Limit = n
	}

	cur, err := DecodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		return req, err
	}
	req.Cursor = cur
	return req, nil
}

// Page is the response envelope returned by every paginated listing.
type Page[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

// NewPage builds a Page from a result set fetched with limit+1 rows: the
// extra row (if present) proves there is a next page without a separate
// COUNT query, and is stripped before returning.
func NewPage[T any](items []T, limit int, cursorOf func(T) Cursor) Page[T] {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	p := Page[T]{Data: items, HasMore: hasMore}
	if hasMore && len(items) > 0 {
		token := cursorOf(items[len(items)-1]).Encode()
		p.NextCursor = &token
	}
	if p.Data == nil {
		p.Data = []T{}
	}
	return p
}
