// Package orchestrator holds the core business logic: it keeps an
// in-memory view of cameras discovered on the media server in sync with
// OBS, and lets an admin manage named positions and which camera (and
// audio) occupies each one.
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
	"unicode/utf8"

	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/mediaserver"
	"live-orchestrator/backend/internal/obs"
	"live-orchestrator/backend/internal/positions"
)

// posInputPrefix prefixes every OBS input created for a position, so it
// never collides with sources the operator created manually in OBS.
const posInputPrefix = "pos_"

// maxPositionNameLength is the maximum allowed length (in runes) of a
// position name after trimming.
const maxPositionNameLength = 100

// offlineRemoveAfter is how long a camera stays "offline" in the map before
// it's pruned entirely. Chosen to absorb brief reconnect blips without
// flicker.
const offlineRemoveAfter = 60 * time.Second

var (
	// ErrCameraNotFound is returned when an operation references an unknown camera ID.
	ErrCameraNotFound = errors.New("câmera não encontrada")
	// ErrCameraOffline is returned when a position operation targets an offline camera.
	ErrCameraOffline = errors.New("câmera está offline")
	// ErrOBSUnreachable is returned when an OBS operation fails because OBS is disconnected.
	ErrOBSUnreachable = errors.New("obs está inacessível")

	// ErrPositionNotFound is returned when an operation references an unknown position ID.
	ErrPositionNotFound = errors.New("posição não encontrada")
	// ErrPositionNameEmpty is returned when a position name is empty after trimming.
	ErrPositionNameEmpty = errors.New("nome de posição não pode ser vazio")
	// ErrPositionNameTooLong is returned when a trimmed position name exceeds maxPositionNameLength.
	ErrPositionNameTooLong = errors.New("nome de posição excede o tamanho máximo permitido")
	// ErrPositionNameTaken is returned when a position name collides with an existing one.
	ErrPositionNameTaken = errors.New("nome de posição já utilizado")
	// ErrPositionOBSInputMissing is returned when a position's expected OBS input doesn't exist.
	ErrPositionOBSInputMissing = errors.New("input do OBS da posição não encontrado")
	// ErrPositionEmpty is returned when an operation requires a camera assigned to the position and none is.
	ErrPositionEmpty = errors.New("posição não possui câmera atribuída")
)

// MediaServerClient is the subset of mediaserver.Client the orchestrator depends on.
type MediaServerClient interface {
	ListActiveStreams(ctx context.Context) ([]mediaserver.StreamInfo, error)
}

// Orchestrator owns the in-memory camera/position state and the sync loop.
type Orchestrator struct {
	mediaClient        MediaServerClient
	obsCtl             obs.Controller
	hub                *events.Hub
	programScene       string
	syncInterval       time.Duration
	mediaSourceBaseURL string
	positionsStore     positions.Store

	mu              sync.RWMutex
	cameras         map[string]*Camera
	offlineSince    map[string]time.Time
	mediaConnected  bool
	positions       map[string]*Position
	audioPositionID string
}

// New creates an Orchestrator, loading any persisted position definitions
// from positionsStore. A missing or corrupt store file is logged and
// treated as an empty position list — it never prevents construction. Call
// Run to start the background sync loop.
func New(mediaClient MediaServerClient, obsCtl obs.Controller, hub *events.Hub, programScene string, syncInterval time.Duration, mediaSourceBaseURL string, positionsStore positions.Store) *Orchestrator {
	o := &Orchestrator{
		mediaClient:        mediaClient,
		obsCtl:             obsCtl,
		hub:                hub,
		programScene:       programScene,
		syncInterval:       syncInterval,
		mediaSourceBaseURL: mediaSourceBaseURL,
		positionsStore:     positionsStore,
		cameras:            make(map[string]*Camera),
		offlineSince:       make(map[string]time.Time),
		positions:          make(map[string]*Position),
	}

	loaded, err := positionsStore.Load()
	if err != nil {
		slog.Warn("failed to load persisted positions, starting with an empty list", "error", err)
		loaded = nil
	}
	for _, p := range loaded {
		o.positions[p.ID] = &Position{ID: p.ID, Name: p.Name}
	}

	return o
}

func positionInputName(id string) string {
	return posInputPrefix + id
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

// Positions returns a snapshot of all known positions, sorted by ID.
func (o *Orchestrator) Positions() []Position {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.positionsSnapshotLocked()
}

func (o *Orchestrator) snapshotLocked() []Camera {
	cams := make([]Camera, 0, len(o.cameras))
	for _, c := range o.cameras {
		cams = append(cams, *c)
	}
	sort.Slice(cams, func(i, j int) bool { return cams[i].ID < cams[j].ID })
	return cams
}

func (o *Orchestrator) positionsSnapshotLocked() []Position {
	ps := make([]Position, 0, len(o.positions))
	for _, p := range o.positions {
		ps = append(ps, *p)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].ID < ps[j].ID })
	return ps
}

func (o *Orchestrator) statusLocked() SystemStatus {
	streaming := false
	for _, p := range o.positions {
		if p.CameraID != "" {
			streaming = true
			break
		}
	}
	return SystemStatus{
		ObsConnected:         o.obsCtl.IsConnected(),
		MediaServerConnected: o.mediaConnected,
		Streaming:            streaming,
		ActiveSceneName:      o.programScene,
	}
}

// persistPositionsLocked saves only the persisted subset (ID/Name) of every
// known position. Must be called with o.mu held.
func (o *Orchestrator) persistPositionsLocked() error {
	defs := make([]positions.Position, 0, len(o.positions))
	for _, p := range o.positions {
		defs = append(defs, positions.Position{ID: p.ID, Name: p.Name})
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	return o.positionsStore.Save(defs)
}

func (o *Orchestrator) publishCameras() {
	o.hub.Publish(events.Event{Type: "cameras.updated", Payload: o.Cameras()})
}

func (o *Orchestrator) publishStatus() {
	o.hub.Publish(events.Event{Type: "system.status", Payload: o.Status()})
}

func (o *Orchestrator) publishPositions() {
	o.hub.Publish(events.Event{Type: "positions.updated", Payload: o.Positions()})
}

func (o *Orchestrator) publishError(message string) {
	o.hub.Publish(events.Event{Type: "error", Payload: map[string]string{"message": message}})
}

// validatePositionName trims name and validates it: non-empty, at most
// maxPositionNameLength runes.
func validatePositionName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ErrPositionNameEmpty
	}
	if utf8.RuneCountInString(trimmed) > maxPositionNameLength {
		return "", ErrPositionNameTooLong
	}
	return trimmed, nil
}

// offlineUnassignment is a position auto-cleared by the offline hook,
// carrying what's needed to apply the matching OBS side effects outside the
// lock.
type offlineUnassignment struct {
	positionID string
	inputName  string
	wasAudio   bool
}

// pendingActions collects side effects to perform outside the lock.
type pendingActions struct {
	removeIDs       []string // camera IDs to prune from the map (grace period elapsed)
	offlineUnassign []offlineUnassignment
}

// clearCameraFromPositionsLocked removes cameraID from whichever position
// currently holds it (there is at most one, by invariant), clearing the
// audio-source flag too if that position was the audio source. Must be
// called with o.mu held for writing.
func (o *Orchestrator) clearCameraFromPositionsLocked(cameraID string) (offlineUnassignment, bool) {
	for id, p := range o.positions {
		if p.CameraID != cameraID {
			continue
		}
		p.CameraID = ""
		wasAudio := p.IsAudioSource
		if wasAudio {
			p.IsAudioSource = false
			o.audioPositionID = ""
		}
		return offlineUnassignment{positionID: id, inputName: positionInputName(id), wasAudio: wasAudio}, true
	}
	return offlineUnassignment{}, false
}

// SyncOnce fetches the active streams from the media server, reconciles the
// in-memory camera map, and auto-clears any position whose camera just went
// offline. It returns the resulting camera snapshot.
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
			if u, ok := o.clearCameraFromPositionsLocked(id); ok {
				actions.offlineUnassign = append(actions.offlineUnassign, u)
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
	for _, u := range actions.offlineUnassign {
		if err := o.obsCtl.SetPositionEnabled(o.programScene, u.inputName, false); err != nil {
			slog.Warn("failed to disable position for offline camera", "position", u.positionID, "error", err)
		}
		if u.wasAudio {
			if err := o.obsCtl.SetInputAudioMuted(u.inputName, true); err != nil {
				slog.Warn("failed to mute position for offline camera", "position", u.positionID, "error", err)
			}
		}
	}

	if len(actions.removeIDs) > 0 {
		o.mu.Lock()
		for _, id := range actions.removeIDs {
			delete(o.cameras, id)
			delete(o.offlineSince, id)
		}
		o.mu.Unlock()
		camsChanged = true
	}

	if camsChanged {
		o.publishCameras()
	}
	if len(actions.offlineUnassign) > 0 {
		o.publishPositions()
		o.publishStatus()
	}

	return o.Cameras()
}

// CreatePosition registers a new named position: it validates the name,
// creates the position's dedicated OBS input eagerly (before the position
// exists anywhere else), then adds it to memory and persists it. If OBS
// input creation fails, the position is never added nor persisted.
func (o *Orchestrator) CreatePosition(name string) (Position, error) {
	trimmed, err := validatePositionName(name)
	if err != nil {
		return Position{}, err
	}

	o.mu.RLock()
	for _, p := range o.positions {
		if p.Name == trimmed {
			o.mu.RUnlock()
			return Position{}, ErrPositionNameTaken
		}
	}
	o.mu.RUnlock()

	id := positions.NewID()
	inputName := positionInputName(id)
	if err := o.obsCtl.CreatePositionInput(o.programScene, inputName); err != nil {
		return Position{}, fmt.Errorf("%w: %v", ErrOBSUnreachable, err)
	}

	o.mu.Lock()
	for _, p := range o.positions {
		if p.Name == trimmed {
			o.mu.Unlock()
			if rmErr := o.obsCtl.RemoveInput(inputName); rmErr != nil {
				slog.Warn("failed to clean up orphaned obs input after name conflict", "input", inputName, "error", rmErr)
			}
			return Position{}, ErrPositionNameTaken
		}
	}
	pos := &Position{ID: id, Name: trimmed}
	o.positions[id] = pos
	if err := o.persistPositionsLocked(); err != nil {
		delete(o.positions, id)
		o.mu.Unlock()
		slog.Warn("failed to persist new position", "error", err)
		return Position{}, err
	}
	result := *pos
	o.mu.Unlock()

	o.publishPositions()
	return result, nil
}

// RenamePosition changes an existing position's name. Renaming to the
// position's current name is a no-op success. It never touches OBS.
func (o *Orchestrator) RenamePosition(id, newName string) (Position, error) {
	trimmed, err := validatePositionName(newName)
	if err != nil {
		return Position{}, err
	}

	o.mu.Lock()
	pos, ok := o.positions[id]
	if !ok {
		o.mu.Unlock()
		return Position{}, ErrPositionNotFound
	}
	changed := pos.Name != trimmed
	if changed {
		for otherID, p := range o.positions {
			if otherID != id && p.Name == trimmed {
				o.mu.Unlock()
				return Position{}, ErrPositionNameTaken
			}
		}
		pos.Name = trimmed
		if err := o.persistPositionsLocked(); err != nil {
			o.mu.Unlock()
			slog.Warn("failed to persist renamed position", "error", err)
			return Position{}, err
		}
	}
	result := *pos
	o.mu.Unlock()

	if changed {
		o.publishPositions()
	}
	return result, nil
}

// DeletePosition removes a position: disables and mutes its OBS input as
// needed, removes the input (best-effort, ADR-003), then removes it from
// memory and persistence.
func (o *Orchestrator) DeletePosition(id string) error {
	o.mu.RLock()
	pos, ok := o.positions[id]
	if !ok {
		o.mu.RUnlock()
		return ErrPositionNotFound
	}
	hadCamera := pos.CameraID != ""
	wasAudio := pos.IsAudioSource
	o.mu.RUnlock()

	inputName := positionInputName(id)
	if hadCamera {
		if err := o.obsCtl.SetPositionEnabled(o.programScene, inputName, false); err != nil {
			slog.Warn("failed to disable position before delete", "position", id, "error", err)
		}
	}
	if wasAudio {
		if err := o.obsCtl.SetInputAudioMuted(inputName, true); err != nil {
			slog.Warn("failed to mute position before delete", "position", id, "error", err)
		}
	}
	if err := o.obsCtl.RemoveInput(inputName); err != nil {
		slog.Warn("failed to remove obs input for deleted position", "position", id, "error", err)
	}

	o.mu.Lock()
	if _, ok := o.positions[id]; !ok {
		o.mu.Unlock()
		return ErrPositionNotFound
	}
	delete(o.positions, id)
	if o.audioPositionID == id {
		o.audioPositionID = ""
	}
	if err := o.persistPositionsLocked(); err != nil {
		o.mu.Unlock()
		slog.Warn("failed to persist position deletion", "error", err)
		return err
	}
	o.mu.Unlock()

	o.publishPositions()
	return nil
}

// AssignCamera assigns cameraID to positionID. If cameraID currently
// occupies a different position, that position is cleared as part of the
// same call (at most one camera per position). Reassigning a camera to the
// position it already occupies is an idempotent no-op.
func (o *Orchestrator) AssignCamera(positionID, cameraID string) (Position, error) {
	o.mu.RLock()
	pos, posOK := o.positions[positionID]
	if !posOK {
		o.mu.RUnlock()
		return Position{}, ErrPositionNotFound
	}
	cam, camOK := o.cameras[cameraID]
	if !camOK {
		o.mu.RUnlock()
		return Position{}, ErrCameraNotFound
	}
	if cam.Status != StatusOnline {
		o.mu.RUnlock()
		return Position{}, ErrCameraOffline
	}
	if pos.CameraID == cameraID {
		result := *pos
		o.mu.RUnlock()
		return result, nil
	}
	sourceURL := cam.SourceURL
	o.mu.RUnlock()

	posInput := positionInputName(positionID)
	if err := o.obsCtl.UpdatePositionSource(posInput, sourceURL); err != nil {
		if errors.Is(err, obs.ErrInputNotFound) {
			return Position{}, ErrPositionOBSInputMissing
		}
		return Position{}, fmt.Errorf("%w: %v", ErrOBSUnreachable, err)
	}
	if err := o.obsCtl.SetPositionEnabled(o.programScene, posInput, true); err != nil {
		return Position{}, fmt.Errorf("%w: %v", ErrOBSUnreachable, err)
	}

	o.mu.Lock()
	pos, posOK = o.positions[positionID]
	if !posOK {
		o.mu.Unlock()
		return Position{}, ErrPositionNotFound
	}
	var bumped offlineUnassignment
	bumpedFound := false
	for otherID, p := range o.positions {
		if otherID == positionID || p.CameraID != cameraID {
			continue
		}
		p.CameraID = ""
		wasAudio := p.IsAudioSource
		if wasAudio {
			p.IsAudioSource = false
			o.audioPositionID = ""
		}
		bumped = offlineUnassignment{positionID: otherID, inputName: positionInputName(otherID), wasAudio: wasAudio}
		bumpedFound = true
		break
	}
	pos.CameraID = cameraID
	result := *pos
	o.mu.Unlock()

	if bumpedFound {
		if err := o.obsCtl.SetPositionEnabled(o.programScene, bumped.inputName, false); err != nil {
			slog.Warn("failed to disable bumped position", "position", bumped.positionID, "error", err)
		}
		if bumped.wasAudio {
			if err := o.obsCtl.SetInputAudioMuted(bumped.inputName, true); err != nil {
				slog.Warn("failed to mute bumped position", "position", bumped.positionID, "error", err)
			}
		}
	}

	o.publishPositions()
	return result, nil
}

// UnassignPosition clears positionID's camera assignment. Unassigning an
// already-empty position is an idempotent no-op. If the position was the
// audio source, that flag is cleared and its input muted.
func (o *Orchestrator) UnassignPosition(positionID string) (Position, error) {
	o.mu.RLock()
	pos, ok := o.positions[positionID]
	if !ok {
		o.mu.RUnlock()
		return Position{}, ErrPositionNotFound
	}
	if pos.CameraID == "" {
		result := *pos
		o.mu.RUnlock()
		return result, nil
	}
	o.mu.RUnlock()

	posInput := positionInputName(positionID)
	if err := o.obsCtl.SetPositionEnabled(o.programScene, posInput, false); err != nil {
		slog.Warn("failed to disable position on unassign", "position", positionID, "error", err)
	}

	o.mu.Lock()
	pos, ok = o.positions[positionID]
	if !ok {
		o.mu.Unlock()
		return Position{}, ErrPositionNotFound
	}
	wasAudio := pos.IsAudioSource
	pos.CameraID = ""
	if wasAudio {
		pos.IsAudioSource = false
		o.audioPositionID = ""
	}
	result := *pos
	o.mu.Unlock()

	if wasAudio {
		if err := o.obsCtl.SetInputAudioMuted(posInput, true); err != nil {
			slog.Warn("failed to mute position on unassign", "position", positionID, "error", err)
		}
	}

	o.publishPositions()
	return result, nil
}

// SetAudioPosition marks positionID as the sole audio source: it mutes the
// previous audio position's input (if any) and unmutes positionID's.
// positionID must currently have a camera assigned.
func (o *Orchestrator) SetAudioPosition(positionID string) (Position, error) {
	o.mu.RLock()
	pos, ok := o.positions[positionID]
	if !ok {
		o.mu.RUnlock()
		return Position{}, ErrPositionNotFound
	}
	if pos.CameraID == "" {
		o.mu.RUnlock()
		return Position{}, ErrPositionEmpty
	}
	prevAudioID := o.audioPositionID
	o.mu.RUnlock()

	posInput := positionInputName(positionID)
	if prevAudioID != "" && prevAudioID != positionID {
		prevInput := positionInputName(prevAudioID)
		if err := o.obsCtl.SetInputAudioMuted(prevInput, true); err != nil {
			slog.Warn("failed to mute previous audio position", "position", prevAudioID, "error", err)
		}
	}
	if err := o.obsCtl.SetInputAudioMuted(posInput, false); err != nil {
		return Position{}, fmt.Errorf("%w: %v", ErrOBSUnreachable, err)
	}

	o.mu.Lock()
	pos, ok = o.positions[positionID]
	if !ok {
		o.mu.Unlock()
		return Position{}, ErrPositionNotFound
	}
	if prevAudioID != "" && prevAudioID != positionID {
		if prevPos, exists := o.positions[prevAudioID]; exists {
			prevPos.IsAudioSource = false
		}
	}
	pos.IsAudioSource = true
	o.audioPositionID = positionID
	result := *pos
	o.mu.Unlock()

	o.publishPositions()
	return result, nil
}
