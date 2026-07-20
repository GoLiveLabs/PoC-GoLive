package positions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileStore_SaveThenLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "positions.json")
	store := NewFileStore(path)

	want := []Position{{ID: "a1", Name: "Principal"}, {ID: "b2", Name: "Secundária"}}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d positions, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("position %d: expected %+v, got %+v", i, want[i], got[i])
		}
	}
}

func TestFileStore_Load_MissingFile_ReturnsEmptySlice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	store := NewFileStore(path)

	got, err := store.Load()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %+v", got)
	}
}

func TestFileStore_Load_InvalidJSON_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "positions.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	store := NewFileStore(path)

	if _, err := store.Load(); err == nil {
		t.Fatalf("expected an error for invalid JSON, got nil")
	}
}

func TestFileStore_Save_OverwritesRatherThanMerges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "positions.json")
	store := NewFileStore(path)

	if err := store.Save([]Position{{ID: "a1", Name: "A"}}); err != nil {
		t.Fatalf("Save A: %v", err)
	}
	if err := store.Save([]Position{{ID: "b2", Name: "B"}}); err != nil {
		t.Fatalf("Save B: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 || got[0].ID != "b2" {
		t.Fatalf("expected only B's content, got %+v", got)
	}
}

func TestNewID_ReturnsDistinctFixedLengthStrings(t *testing.T) {
	id1 := NewID()
	id2 := NewID()

	if id1 == "" || id2 == "" {
		t.Fatalf("expected non-empty IDs, got %q and %q", id1, id2)
	}
	if id1 == id2 {
		t.Fatalf("expected distinct IDs, got the same value twice: %q", id1)
	}
	if len(id1) != len(id2) {
		t.Fatalf("expected IDs of the same length, got %d and %d", len(id1), len(id2))
	}
}

// TestFileStore_Save_AtomicWriteFailureLeavesPriorContentIntact simulates a
// failure during the atomic-rename step (UT-050) by pointing Save at a path
// whose parent directory does not allow creating the temp file used for the
// rename, after a first successful Save. Load afterward must still return
// the last successfully saved content, never a truncated or corrupted file.
func TestFileStore_Save_AtomicWriteFailureLeavesPriorContentIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "positions.json")
	store := NewFileStore(path)

	original := []Position{{ID: "a1", Name: "Principal"}}
	if err := store.Save(original); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	// Make the destination file read-only so the atomic rename step fails
	// after the temp file has already been written, without touching the
	// destination's existing content.
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(path, 0o644)

	err := store.Save([]Position{{ID: "b2", Name: "Secundária"}})
	if err == nil {
		t.Fatalf("expected Save to fail while the destination file is read-only")
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod restore: %v", err)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading file after failed save: %v", readErr)
	}
	var got []Position
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if len(got) != 1 || got[0] != original[0] {
		t.Fatalf("expected prior content intact, got %+v", got)
	}
}
