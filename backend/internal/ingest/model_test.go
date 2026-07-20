package ingest

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestNew_ProtocolDerivedFromScheme(t *testing.T) {
	cases := map[string]Protocol{
		"http://acme.com/feed":     ProtocolHTTP,
		"https://acme.com/feed":    ProtocolHTTPS,
		"ftp://acme.com/feed":      ProtocolFTP,
		"sftp://files.acme.com/in": ProtocolSFTP,
		"s3://bucket/key":          ProtocolS3,
	}
	for rawURL, want := range cases {
		ing, err := New(uuid.New(), rawURL, true)
		if err != nil {
			t.Fatalf("New(%q): %v", rawURL, err)
		}
		if Protocol(ing.Protocol) != want {
			t.Fatalf("New(%q): got protocol %q, want %q", rawURL, ing.Protocol, want)
		}
	}
}

func TestNew_UnsupportedScheme_Rejected(t *testing.T) {
	if _, err := New(uuid.New(), "gopher://acme.com/feed", true); err == nil {
		t.Fatal("expected an error for unsupported scheme")
	}
}

func TestNew_MissingScheme_Rejected(t *testing.T) {
	if _, err := New(uuid.New(), "acme.com/feed", true); !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("expected ErrInvalidURL, got %v", err)
	}
}

func TestNew_MissingHost_Rejected(t *testing.T) {
	if _, err := New(uuid.New(), "https:///feed", true); !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("expected ErrInvalidURL, got %v", err)
	}
}

func TestNew_EmbeddedCredentials_Rejected(t *testing.T) {
	if _, err := New(uuid.New(), "sftp://user:pass@files.acme.com/inbox", true); !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("expected ErrInvalidURL for embedded credentials, got %v", err)
	}
}

func TestNew_NormalizesSchemeAndHostToLowercase(t *testing.T) {
	ing, err := New(uuid.New(), "HTTPS://ACME.com/Feed", true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if ing.URL != "https://acme.com/Feed" {
		t.Fatalf("expected scheme/host normalized to lowercase, got %q", ing.URL)
	}
}

func TestChangeURL_RederivesProtocol(t *testing.T) {
	ing, err := New(uuid.New(), "https://acme.com/feed", true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := ing.ChangeURL("sftp://files.acme.com/inbox"); err != nil {
		t.Fatalf("ChangeURL: %v", err)
	}
	if Protocol(ing.Protocol) != ProtocolSFTP {
		t.Fatalf("expected protocol re-derived to sftp, got %q", ing.Protocol)
	}
	if ing.URL != "sftp://files.acme.com/inbox" {
		t.Fatalf("unexpected URL after ChangeURL: %q", ing.URL)
	}
}
