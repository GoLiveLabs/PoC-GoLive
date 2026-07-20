package orchestrator

import "time"

// Camera is the shared REST/WebSocket contract for a single camera.
type Camera struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	SourceURL  string    `json:"sourceUrl"`
	Status     string    `json:"status"` // "online" | "offline"
	LastSeenAt time.Time `json:"lastSeenAt"`
}

const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// Position is the shared REST/WebSocket contract for a named position: its
// persisted identity (ID/Name) plus in-memory-only runtime state (which
// camera currently occupies it, whether it's the audio source).
type Position struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CameraID      string `json:"cameraId"` // "" when unassigned
	IsAudioSource bool   `json:"isAudioSource"`
}

// SystemStatus is the shared REST/WebSocket contract for overall system state.
type SystemStatus struct {
	ObsConnected         bool   `json:"obsConnected"`
	MediaServerConnected bool   `json:"mediaServerConnected"`
	Streaming            bool   `json:"streaming"`
	ActiveSceneName      string `json:"activeSceneName"`
}
