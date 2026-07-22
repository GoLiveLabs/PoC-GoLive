package scenes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// UT-001
func TestFileStore_SaveThenLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scenes.json")
	store := NewFileStore(path)

	want := []Scene{
		{ID: "a1", Name: "Abertura", PositionIDs: []string{"p1", "p2"}},
		{ID: "b2", Name: "Entrevista", PositionIDs: []string{"p3"}},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

// UT-002
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

// UT-003
func TestFileStore_Save_UnwritablePath_ReturnsError(t *testing.T) {
	// Point at a path whose parent is a regular file so MkdirAll/CreateTemp fail.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing blocker: %v", err)
	}
	store := NewFileStore(filepath.Join(blocker, "scenes.json"))

	err := store.Save([]Scene{{ID: "a1", Name: "A", PositionIDs: nil}})
	if err == nil {
		t.Fatalf("expected Save to fail on unwritable path")
	}
}

// UT-004
func TestFileStore_Save_EmptySlice_WritesEmptyJSONArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scenes.json")
	store := NewFileStore(path)

	if err := store.Save([]Scene{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("file is not valid JSON: %v", err)
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected JSON array, got %T", raw)
	}
	if len(arr) != 0 {
		t.Fatalf("expected empty JSON array, got %v", arr)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %+v", got)
	}
}
