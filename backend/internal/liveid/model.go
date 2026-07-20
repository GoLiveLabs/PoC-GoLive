// Package liveid binds a client to the identifier a streaming platform uses
// for one of its live streams (a YouTube video id, a Twitch channel id, ...).
// A client can hold several of these on the same platform — one per stream —
// so this is a list, not a single slot per (client, platform).
package liveid

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxLiveIDLen = 255

var (
	ErrNotFound         = errors.New("live id not found")
	ErrDuplicateLiveID  = errors.New("this live id already exists for the client and platform")
	ErrClientNotFound   = errors.New("client not found")
	ErrPlatformNotFound = errors.New("streaming platform not found")
	ErrInvalidLiveID    = errors.New("live id must not be empty and at most 255 characters")
)

// ClientLiveID is the aggregate.
type ClientLiveID struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ClientID   uuid.UUID `gorm:"type:uuid;not null;index:idx_live_ids_tuple,unique"`
	PlatformID uuid.UUID `gorm:"type:uuid;not null;index:idx_live_ids_tuple,unique"`
	LiveID     string    `gorm:"type:text;not null;index:idx_live_ids_tuple,unique"`
	// See internal/ingest.Ingest.IsActive for why there's no `default:` tag.
	IsActive  bool `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (ClientLiveID) TableName() string { return "client_live_ids" }

// New validates invariants at construction.
func New(clientID, platformID uuid.UUID, liveID string, isActive bool) (*ClientLiveID, error) {
	liveID, err := normalizeLiveID(liveID)
	if err != nil {
		return nil, err
	}
	return &ClientLiveID{
		ClientID:   clientID,
		PlatformID: platformID,
		LiveID:     liveID,
		IsActive:   isActive,
	}, nil
}

func normalizeLiveID(liveID string) (string, error) {
	liveID = strings.TrimSpace(liveID)
	if liveID == "" || len(liveID) > maxLiveIDLen {
		return "", ErrInvalidLiveID
	}
	return liveID, nil
}
