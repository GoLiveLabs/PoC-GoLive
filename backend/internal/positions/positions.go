// Package positions persists named-position definitions (id/name only)
// between backend restarts. All runtime state — which camera occupies a
// position, which position is the audio source — lives in the orchestrator
// package's in-memory maps and is never persisted here (ADR-002).
package positions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Position is the persisted definition of a named position: identity and
// display name only.
type Position struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var (
	// ErrNotFound is returned when an operation references an unknown position ID.
	ErrNotFound = errors.New("posição não encontrada")
	// ErrNameTaken is returned when a position name collides with an existing one.
	ErrNameTaken = errors.New("nome de posição já utilizado")
)

// Store persists and loads position definitions.
type Store interface {
	Load() ([]Position, error)
	Save(positions []Position) error
}

// idHexChars is the fixed length (in hex characters) of every generated ID.
const idHexChars = 16

// NewID returns a new crypto/rand-backed, fixed-length hex identifier.
func NewID() string {
	b := make([]byte, idHexChars/2)
	if _, err := rand.Read(b); err != nil {
		panic("positions: failed to read random bytes: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// FileStore persists positions as a JSON array in a single file on disk,
// writing atomically via a temp-file-then-rename so a crash mid-write never
// corrupts the existing file.
type FileStore struct {
	path string
}

// NewFileStore creates a FileStore backed by the file at path. The file and
// its parent directory do not need to exist yet.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Load reads the positions file. A missing file is not an error — it
// returns an empty slice, matching the "first use" case. A file that exists
// but contains invalid JSON returns a non-nil error.
func (f *FileStore) Load() ([]Position, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Position{}, nil
		}
		return nil, err
	}
	var loaded []Position
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}
	return loaded, nil
}

// Save writes positions to disk atomically: it writes to a temp file in the
// same directory, then renames it over the destination path. If any step
// before the rename fails, the destination file is left untouched.
func (f *FileStore) Save(positions []Position) error {
	data, err := json.MarshalIndent(positions, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".positions-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, f.path)
}
