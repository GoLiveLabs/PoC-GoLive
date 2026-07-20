package pagination

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursor_EncodeDecode_RoundTrip(t *testing.T) {
	c := Cursor{CreatedAt: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC), ID: uuid.New()}
	token := c.Encode()

	got, err := DecodeCursor(token)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if !got.CreatedAt.Equal(c.CreatedAt) || got.ID != c.ID {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, c)
	}
}

func TestDecodeCursor_Empty(t *testing.T) {
	c, err := DecodeCursor("")
	if err != nil || c != nil {
		t.Fatalf("expected (nil, nil) for empty cursor, got (%v, %v)", c, err)
	}
}

func TestDecodeCursor_Invalid(t *testing.T) {
	if _, err := DecodeCursor("not-valid-base64!!"); err != ErrInvalidCursor {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestParseRequest_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	req, err := ParseRequest(r)
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	if req.Limit != DefaultLimit || req.Cursor != nil {
		t.Fatalf("unexpected defaults: %+v", req)
	}
}

func TestParseRequest_LimitCappedAtMax(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?limit=99999", nil)
	req, err := ParseRequest(r)
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	if req.Limit != MaxLimit {
		t.Fatalf("expected limit capped at %d, got %d", MaxLimit, req.Limit)
	}
}

func TestParseRequest_InvalidLimit(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?limit=0", nil)
	if _, err := ParseRequest(r); err != ErrInvalidLimit {
		t.Fatalf("expected ErrInvalidLimit, got %v", err)
	}
}

func TestNewPage_HasMore(t *testing.T) {
	items := []int{1, 2, 3}
	page := NewPage(items, 2, func(i int) Cursor {
		return Cursor{CreatedAt: time.Unix(int64(i), 0), ID: uuid.New()}
	})
	if !page.HasMore {
		t.Fatal("expected HasMore true")
	}
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 items after trimming, got %d", len(page.Data))
	}
	if page.NextCursor == nil {
		t.Fatal("expected NextCursor to be set")
	}
}

func TestNewPage_NoMore(t *testing.T) {
	items := []int{1, 2}
	page := NewPage(items, 2, func(i int) Cursor {
		return Cursor{CreatedAt: time.Unix(int64(i), 0), ID: uuid.New()}
	})
	if page.HasMore {
		t.Fatal("expected HasMore false")
	}
	if page.NextCursor != nil {
		t.Fatal("expected NextCursor nil")
	}
}

func TestNewPage_EmptyNotNull(t *testing.T) {
	page := NewPage([]int(nil), 10, func(i int) Cursor { return Cursor{} })
	if page.Data == nil {
		t.Fatal("expected Data to be an empty slice, not nil")
	}
}
