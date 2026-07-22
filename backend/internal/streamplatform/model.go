// Package streamplatform manages the streaming platform catalog (YouTube,
// Twitch, TikTok, ...). Unlike ingest.Protocol this is a lookup table, not an
// enum: adding a new platform is a plain insert, not a migration. The
// catalog is shared globally, not scoped to any client.
package streamplatform

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxSlugLen = 100
	maxNameLen = 200
)

var (
	ErrNotFound      = errors.New("streaming platform not found")
	ErrDuplicateSlug = errors.New("streaming platform already exists")
	ErrInvalidSlug   = errors.New("slug must not be empty and at most 100 characters")
	ErrInvalidName   = errors.New("display name must not be empty and at most 200 characters")
	ErrPlatformInUse = errors.New("streaming platform is referenced by existing live ids")
)

// Platform is a row in the streaming platform catalog.
type Platform struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug              string    `gorm:"type:text;not null;uniqueIndex"`
	DisplayName       string    `gorm:"type:text;not null"`
	IngestURLTemplate string    `gorm:"type:text;not null;default:''"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Platform) TableName() string { return "streaming_platforms" }

// New validates invariants at construction. Slug is normalized to lower case
// since it's the stable, machine-facing identifier; DisplayName keeps its
// casing since it's what a human sees.
func New(slug, displayName string) (*Platform, error) {
	slug = normalizeSlug(slug)
	if slug == "" || len(slug) > maxSlugLen {
		return nil, ErrInvalidSlug
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > maxNameLen {
		return nil, ErrInvalidName
	}
	return &Platform{Slug: slug, DisplayName: displayName}, nil
}

func normalizeSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}
