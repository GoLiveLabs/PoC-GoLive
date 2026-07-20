// Package ingest manages a client's media ingestion sources: a URL whose
// protocol is derived from its own scheme, never accepted as a separate
// field (two sources of truth for the same fact always drift).
package ingest

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxURLLen = 2048

var (
	ErrNotFound         = errors.New("ingest not found")
	ErrDuplicateURL     = errors.New("ingest with this url already exists for the client")
	ErrClientNotFound   = errors.New("client not found")
	ErrInvalidURL       = errors.New("url is not valid")
	ErrUnsupportedProto = errors.New("unsupported protocol")
)

// Protocol is a value object over a closed set of schemes. Validated in one
// place beats a bare string validated everywhere it's used.
type Protocol string

const (
	ProtocolHTTP  Protocol = "http"
	ProtocolHTTPS Protocol = "https"
	ProtocolFTP   Protocol = "ftp"
	ProtocolSFTP  Protocol = "sftp"
	ProtocolS3    Protocol = "s3"
)

var validProtocols = map[Protocol]struct{}{
	ProtocolHTTP: {}, ProtocolHTTPS: {}, ProtocolFTP: {}, ProtocolSFTP: {}, ProtocolS3: {},
}

func ParseProtocol(s string) (Protocol, error) {
	p := Protocol(strings.ToLower(strings.TrimSpace(s)))
	if _, ok := validProtocols[p]; !ok {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedProto, s)
	}
	return p, nil
}

func (p Protocol) String() string { return string(p) }

// Ingest is the aggregate.
type Ingest struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ClientID uuid.UUID `gorm:"type:uuid;not null;index:idx_ingests_client_url,unique"`
	URL      string    `gorm:"type:text;not null;index:idx_ingests_client_url,unique"`
	Protocol string    `gorm:"type:text;not null"`
	// No `default:` tag: GORM omits a field from INSERT when it holds its Go
	// zero value under a `default` tag, which would silently turn an explicit
	// IsActive:false back into the column's DB-level default (true). The
	// default is applied in application code (CreateRequest.ActiveOrDefault),
	// not here.
	IsActive  bool `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Ingest) TableName() string { return "ingests" }

// New derives the protocol from the URL scheme rather than trusting a
// separate field.
func New(clientID uuid.UUID, rawURL string, isActive bool) (*Ingest, error) {
	normalized, proto, err := parseAndValidateURL(rawURL)
	if err != nil {
		return nil, err
	}
	return &Ingest{
		ClientID: clientID,
		URL:      normalized,
		Protocol: proto.String(),
		IsActive: isActive,
	}, nil
}

// parseAndValidateURL normalizes the URL and extracts its protocol.
func parseAndValidateURL(rawURL string) (string, Protocol, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || len(rawURL) > maxURLLen {
		return "", "", ErrInvalidURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}
	if u.Scheme == "" {
		return "", "", fmt.Errorf("%w: missing scheme", ErrInvalidURL)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("%w: missing host", ErrInvalidURL)
	}

	proto, err := ParseProtocol(u.Scheme)
	if err != nil {
		return "", "", err
	}

	// Credentials in a URL end up in logs, error messages and the database in
	// plaintext. Reject them at the door.
	if u.User != nil {
		return "", "", fmt.Errorf("%w: credentials must not be embedded in the url", ErrInvalidURL)
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String(), proto, nil
}

// ChangeURL keeps URL and Protocol consistent on mutation.
func (i *Ingest) ChangeURL(rawURL string) error {
	normalized, proto, err := parseAndValidateURL(rawURL)
	if err != nil {
		return err
	}
	i.URL = normalized
	i.Protocol = proto.String()
	return nil
}
