// Package orchestrator holds the core business logic: it keeps an
// in-memory view of cameras discovered on the media server in sync with
// OBS inputs, and lets the operator pick which camera is live.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/mediaserver"
	"live-orchestrator/backend/internal/obs"
)

// CamPrefix mirrors obs.CamPrefix; duplicated as a const here would risk
// drift, so we reference the OBS package's constant directly where needed.
const CamPrefix = obs.CamPrefix

// offlineRemoveAfter is how long a camera stays "offline" in the map before
// its OBS input is torn down. Chosen to absorb brief reconnect blips without
// flicker in OBS.
const offlineRemoveAfter = 60 * time.Second

var (
	// ErrCameraNotFound is returned when an operation references an unknown camera ID.
	ErrCameraNotFound = errors.New("camera not found")
	// ErrCameraOffline is returned when SetLive is called on an offline camera.
	ErrCameraOffline = errors.New("camera is offline")
	// ErrOBSUnreachable is returned when an OBS operation fails because OBS is disconnected.
	ErrOBSUnreachable = errors.New("obs is unreachable")
)

// MediaServerClient is the subset of mediaserver.Client the orchestrator depends on.
type MediaServerClient interface {
	ListActiveStreams(ctx context.Context) ([]mediaserver.StreamInfo, error)
}

// Orchestrator owns the in-memory camera state and the sync loop.
type Orchestrator struct {
	mediaClient        MediaServerClient
	obsCtl             obs.Controller
	hub                *events.Hub
	programScene       string
	syncInterval       time.Duration
	mediaSourceBaseURL string

	mu             sync.RWMutex
	cameras        map[string]*Camera
	offlineSince   map[string]time.Time
	liveCameraID   string
	mediaConnected bool
}

// New creates an Orchestrator. Call Run to start the background sync loop.
func New(mediaClient MediaServerClient, obsCtl obs.Controller, hub *events.Hub, programScene string, syncInterval time.Duration, mediaSourceBaseURL string) *Orchestrator {
	return &Orchestrator{
		mediaClient:        mediaClient,
		obsCtl:             obsCtl,
		hub:                hub,
		programScene:       programScene,
		syncInterval:       syncInterval,
		mediaSourceBaseURL: mediaSourceBaseURL,
		cameras:            make(map[string]*Camera),
		offlineSince:       make(map[string]time.Time),
	}
}

// Run starts the periodic sync loop. It blocks until ctx is cancelled.
func (o *Orchestrator) Run(ctx context.Context) {
	if err := o.obsCtl.EnsureScene(o.programScene); err != nil {
		slog.Warn("could not ensure program scene on startup", "scene", o.programScene, "error", err)
	}

	ticker := time.NewTicker(o.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.SyncOnce(ctx)
		}
	}
}

// Cameras returns a snapshot of all known cameras, sorted by ID.
func (o *Orchestrator) Cameras() []Camera {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.snapshotLocked()
}

// Status returns the current SystemStatus.
func (o *Orchestrator) Status() SystemStatus {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.statusLocked()
}

func (o *Orchestrator) snapshotLocked() []Camera {
	cams := make([]Camera, 0, len(o.cameras))
	for _, c := range o.cameras {
		cams = append(cams, *c)
	}
	sort.Slice(cams, func(i, j int) bool { return cams[i].ID < cams[j].ID })
	return cams
}

func (o *Orchestrator) statusLocked() SystemStatus {
	return SystemStatus{
		ObsConnected:         o.obsCtl.IsConnected(),
		MediaServerConnected: o.mediaConnected,
		Streaming:            o.liveCameraID != "",
		ActiveSceneName:      o.programScene,
		LiveCameraID:         o.liveCameraID,
	}
}

func (o *Orchestrator) publishCameras() {
	o.hub.Publish(events.Event{Type: "cameras.updated", Payload: o.Cameras()})
}

func (o *Orchestrator) publishStatus() {
	o.hub.Publish(events.Event{Type: "system.status", Payload: o.Status()})
}

func (o *Orchestrator) publishError(message string) {
	o.hub.Publish(events.Event{Type: "error", Payload: map[string]string{"message": message}})
}

// pendingActions collects side effects to perform outside the lock.
type pendingActions struct {
	createInputs []Camera // cameras needing an OBS input created/updated
	removeIDs    []string // camera IDs to remove from OBS + the map
	liveLost     bool     // the live camera just dropped
}

// SyncOnce fetches the active streams from the media server, reconciles the
// in-memory camera map, and applies the necessary OBS input changes. It
// returns the resulting camera snapshot.
func (o *Orchestrator) SyncOnce(ctx context.Context) []Camera {
	streams, err := o.mediaClient.ListActiveStreams(ctx)

	o.mu.Lock()
	if err != nil {
		wasConnected := o.mediaConnected
		o.mediaConnected = false
		o.mu.Unlock()
		if wasConnected {
			slog.Warn("lost connection to media server", "error", err)
			o.publishError("Não foi possível conectar ao servidor de mídia.")
			o.publishStatus()
		}
		return o.Cameras()
	}
	o.mediaConnected = true

	now := time.Now()
	seen := make(map[string]bool, len(streams))
	var actions pendingActions
	camsChanged := false

	for _, s := range streams {
		seen[s.Name] = true
		cam, exists := o.cameras[s.Name]
		if !exists {
			cam = &Camera{
				ID:         s.Name,
				Name:       s.Name,
				SourceURL:  fmt.Sprintf("%s/%s", strings.TrimRight(o.mediaSourceBaseURL, "/"), s.Name),
				Status:     StatusOnline,
				LastSeenAt: now,
			}
			o.cameras[s.Name] = cam
			camsChanged = true
		} else {
			if cam.Status != StatusOnline {
				cam.Status = StatusOnline
				delete(o.offlineSince, s.Name)
				camsChanged = true
			}
			cam.LastSeenAt = now
		}
		if !cam.ObsSourceCreated {
			actions.createInputs = append(actions.createInputs, *cam)
		}
	}

	// Cameras that vanished from this sync's stream list.
	for id, cam := range o.cameras {
		if seen[id] {
			continue
		}
		if cam.Status == StatusOnline {
			cam.Status = StatusOffline
			o.offlineSince[id] = now
			camsChanged = true
			if cam.IsLive {
				actions.liveLost = true
			}
			continue
		}
		since, ok := o.offlineSince[id]
		if ok && now.Sub(since) >= offlineRemoveAfter {
			actions.removeIDs = append(actions.removeIDs, id)
		}
	}
	o.mu.Unlock()

	// Apply OBS side effects outside the lock.
	for _, cam := range actions.createInputs {
		inputName := CamPrefix + cam.ID
		if err := o.obsCtl.CreateCameraInput(o.programScene, inputName, cam.SourceURL); err != nil {
			slog.Warn("failed to create obs input for camera", "camera", cam.ID, "error", err)
			continue
		}
		o.mu.Lock()
		if c, ok := o.cameras[cam.ID]; ok {
			c.ObsSourceCreated = true
			camsChanged = true
		}
		o.mu.Unlock()
	}

	for _, id := range actions.removeIDs {
		inputName := CamPrefix + id
		if err := o.obsCtl.RemoveInput(inputName); err != nil {
			slog.Warn("failed to remove obs input for camera", "camera", id, "error", err)
			continue
		}
		o.mu.Lock()
		wasLive := o.cameras[id] != nil && o.cameras[id].IsLive
		delete(o.cameras, id)
		delete(o.offlineSince, id)
		if wasLive {
			o.liveCameraID = ""
		}
		o.mu.Unlock()
		camsChanged = true
	}

	if camsChanged {
		o.publishCameras()
	}
	if actions.liveLost {
		o.publishError("A câmera ao vivo ficou offline. Escolha outra câmera para o ar.")
	}
	if actions.liveLost || len(actions.removeIDs) > 0 {
		o.publishStatus()
	}

	return o.Cameras()
}

// SetLive makes cameraID the visible source in the program scene and hides
// every other camera. It fails with ErrCameraNotFound, ErrCameraOffline, or
// ErrOBSUnreachable as appropriate.
func (o *Orchestrator) SetLive(cameraID string) (SystemStatus, error) {
	o.mu.RLock()
	cam, ok := o.cameras[cameraID]
	if !ok {
		o.mu.RUnlock()
		return SystemStatus{}, ErrCameraNotFound
	}
	if cam.Status != StatusOnline {
		o.mu.RUnlock()
		return SystemStatus{}, ErrCameraOffline
	}
	o.mu.RUnlock()

	if !o.obsCtl.IsConnected() {
		return SystemStatus{}, ErrOBSUnreachable
	}

	inputName := CamPrefix + cameraID
	if err := o.obsCtl.SetOnlyVisibleSource(o.programScene, inputName); err != nil {
		return SystemStatus{}, fmt.Errorf("%w: %v", ErrOBSUnreachable, err)
	}

	o.mu.Lock()
	for id, c := range o.cameras {
		c.IsLive = id == cameraID
	}
	o.liveCameraID = cameraID
	status := o.statusLocked()
	o.mu.Unlock()

	o.publishCameras()
	o.publishStatus()

	return status, nil
}
