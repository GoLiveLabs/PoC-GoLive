// Package client manages the tenant's end customers: name + optional email.
// Ingests and live ids attach to a client.
package client

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	maxNameLen  = 200
	maxEmailLen = 320
)

// emailPattern is a deliberately loose "looks like an email" check (mirrors
// what a validator library's "email" tag does) — full RFC 5322 compliance is
// not the goal, catching obviously malformed input is.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// Domain errors. The service speaks these; only the HTTP layer knows about status codes.
var (
	ErrNotFound      = errors.New("client not found")
	ErrDuplicateName = errors.New("client with this name already exists")
	ErrInvalidName   = errors.New("client name must not be empty and at most 200 characters")
	ErrInvalidEmail  = errors.New("email must be a valid address of at most 320 characters")
)

// Client is the aggregate. Fields are exported because this package is the
// boundary — nothing outside internal/client constructs one directly.
type Client struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string    `gorm:"type:text;not null"`
	Email     *string   `gorm:"type:text"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (Client) TableName() string { return "clients" }

// New validates invariants at construction. A Client that exists is valid.
func New(name string, email *string) (*Client, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxNameLen {
		return nil, ErrInvalidName
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	return &Client{Name: name, Email: email}, nil
}

// normalizeEmail trims the value and validates its format; an empty (after
// trim) input is treated as "no email", not a validation error.
func normalizeEmail(email *string) (*string, error) {
	if email == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*email)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > maxEmailLen || !emailPattern.MatchString(trimmed) {
		return nil, ErrInvalidEmail
	}
	return &trimmed, nil
}
