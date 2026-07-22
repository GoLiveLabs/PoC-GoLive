// Package scenes persists named-scene definitions (id/name/positionIds)
// between backend restarts, mirroring internal/positions (ADR-006).
package scenes

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// Scene is the persisted definition of a named scene: identity, display
// name, and the ordered set of position IDs it references.
type Scene struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	PositionIDs []string `json:"positionIds"`
}

// Store persists and loads scene definitions.
type Store interface {
	Load() ([]Scene, error)
	Save(scenes []Scene) error
}

const idHexChars = 16

// NewID returns a new crypto/rand-backed, fixed-length hex identifier.
func NewID() string {
	b := make([]byte, idHexChars/2)
	if _, err := rand.Read(b); err != nil {
		panic("scenes: failed to read random bytes: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// FileStore persists scenes as a JSON array in a single file on disk,
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

// Load reads the scenes file. A missing file is not an error — it returns
// an empty slice, matching the "first use" case. A file that exists but
// contains invalid JSON returns a non-nil error.
func (f *FileStore) Load() ([]Scene, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Scene{}, nil
		}
		return nil, err
	}
	var loaded []Scene
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}
	if loaded == nil {
		loaded = []Scene{}
	}
	return loaded, nil
}

// Save writes scenes to disk atomically: it writes to a temp file in the
// same directory, then renames it over the destination path. If any step
// before the rename fails, the destination file is left untouched.
func (f *FileStore) Save(scenes []Scene) error {
	if scenes == nil {
		scenes = []Scene{}
	}
	data, err := json.MarshalIndent(scenes, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".scenes-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, f.path)
}
