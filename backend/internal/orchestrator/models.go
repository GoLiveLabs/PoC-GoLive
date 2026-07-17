package orchestrator

import "time"

// Camera is the shared REST/WebSocket contract for a single camera.
type Camera struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	SourceURL        string    `json:"sourceUrl"`
	Status           string    `json:"status"` // "online" | "offline"
	ObsSourceCreated bool      `json:"obsSourceCreated"`
	IsLive           bool      `json:"isLive"`
	LastSeenAt       time.Time `json:"lastSeenAt"`
}

const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// SystemStatus is the shared REST/WebSocket contract for overall system state.
type SystemStatus struct {
	ObsConnected         bool   `json:"obsConnected"`
	MediaServerConnected bool   `json:"mediaServerConnected"`
	Streaming            bool   `json:"streaming"`
	ActiveSceneName      string `json:"activeSceneName"`
	LiveCameraID         string `json:"liveCameraId"`
}
