package liveid_test

import (
	"strings"
	"testing"

	"live-orchestrator/backend/internal/liveid"
)

// UT-074
func TestToResponse_LongStreamKey_MasksAllButLast4(t *testing.T) {
	const key = "xxxx-yyyy-miu9"
	resp := liveid.ToResponse(&liveid.ClientLiveID{StreamKey: key})
	if strings.Contains(resp.StreamKey, "xxxx") || strings.Contains(resp.StreamKey, "yyyy") {
		t.Fatalf("masked key leaked cleartext: %q", resp.StreamKey)
	}
	if !strings.HasSuffix(resp.StreamKey, "miu9") {
		t.Fatalf("expected suffix miu9, got %q", resp.StreamKey)
	}
	// Spec example form "****miu9" is acceptable; full-length mask also ok.
	for _, r := range resp.StreamKey[:len(resp.StreamKey)-4] {
		if r != '*' {
			t.Fatalf("expected '*' prefix, got %q", resp.StreamKey)
		}
	}
}

// UT-075
func TestToResponse_ShortStreamKey_FullyMasked(t *testing.T) {
	resp := liveid.ToResponse(&liveid.ClientLiveID{StreamKey: "ab"})
	if resp.StreamKey != "**" {
		t.Fatalf("expected %q, got %q", "**", resp.StreamKey)
	}
}
