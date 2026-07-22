// Package orchestrator holds the core business logic: it keeps an
// in-memory view of cameras discovered on the media server in sync with
// OBS, and lets an admin manage named positions, scenes, and what's on air.
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
	"live-orchestrator/backend/internal/scenes"
)

// posInputPrefix prefixes every OBS input created for a position, so it
// never collides with sources the operator created manually in OBS.
const posInputPrefix = "pos_"

// simplePositionID is the reserved hidden position used for modo simples.
const simplePositionID = "__simple__"

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

	// ErrSceneNotFound is returned when an operation references an unknown scene ID.
	ErrSceneNotFound = errors.New("cena não encontrada")
	// ErrSceneNameRequired is returned when a scene name is empty after trimming.
	ErrSceneNameRequired = errors.New("nome de cena é obrigatório")
	// ErrSceneNameTaken is returned when a scene name collides with an existing one.
	ErrSceneNameTaken = errors.New("nome de cena já utilizado")
	// ErrSceneIsLive is returned when deleting the currently live scene.
	ErrSceneIsLive = errors.New("cena está ao vivo")
	// ErrReservedPosition is returned when an operation targets the hidden simple-mode position.
	ErrReservedPosition = errors.New("posição reservada")
	// ErrPreviewEmpty is returned when Cut is called with nothing in preview.
	ErrPreviewEmpty = errors.New("nada em prévia")
	// ErrSourceUnavailable is returned when Cut targets an offline source.
	ErrSourceUnavailable = errors.New("fonte indisponível")
)

// MediaServerClient is the subset of mediaserver.Client the orchestrator depends on.
type MediaServerClient interface {
	ListActiveStreams(ctx context.Context) ([]mediaserver.StreamInfo, error)
}

// Orchestrator owns the in-memory camera/position/scene/live state and the sync loop.
type Orchestrator struct {
	mediaClient        MediaServerClient
	obsCtl             obs.Controller
	hub                *events.Hub
	programScene       string
	syncInterval       time.Duration
	mediaSourceBaseURL string
	positionsStore     positions.Store
	scenesStore        scenes.Store

	mu              sync.RWMutex
	cameras         map[string]*Camera
	offlineSince    map[string]time.Time
	mediaConnected  bool
	positions       map[string]*Position
	scenes          map[string]*Scene
	audioPositionID string
	live            LiveState
	hiddenReady     bool
}

// New creates an Orchestrator, loading persisted positions and scenes.
// A missing or corrupt store file is logged and treated as empty.
func New(mediaClient MediaServerClient, obsCtl obs.Controller, hub *events.Hub, programScene string, syncInterval time.Duration, mediaSourceBaseURL string, positionsStore positions.Store, scenesStore scenes.Store) *Orchestrator {
	o := &Orchestrator{
		mediaClient:        mediaClient,
		obsCtl:             obsCtl,
		hub:                hub,
		programScene:       programScene,
		syncInterval:       syncInterval,
		mediaSourceBaseURL: mediaSourceBaseURL,
		positionsStore:     positionsStore,
		scenesStore:        scenesStore,
		cameras:            make(map[string]*Camera),
		offlineSince:       make(map[string]time.Time),
		positions:          make(map[string]*Position),
		scenes:             make(map[string]*Scene),
	}

	loaded, err := positionsStore.Load()
	if err != nil {
		slog.Warn("failed to load persisted positions, starting with an empty list", "error", err)
		loaded = nil
	}
	for _, p := range loaded {
		if p.ID == simplePositionID {
			continue
		}
		o.positions[p.ID] = &Position{ID: p.ID, Name: p.Name}
	}

	loadedScenes, err := scenesStore.Load()
	if err != nil {
		slog.Warn("failed to load persisted scenes, starting with an empty list", "error", err)
		loadedScenes = nil
	}
	for _, s := range loadedScenes {
		ids := append([]string(nil), s.PositionIDs...)
		if ids == nil {
			ids = []string{}
		}
		o.scenes[s.ID] = &Scene{ID: s.ID, Name: s.Name, PositionIDs: ids}
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
	if err := o.ensureHiddenPosition(); err != nil {
		slog.Warn("could not ensure hidden simple-mode position on startup", "error", err)
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

// Positions returns a snapshot of all known public positions, sorted by ID.
// The reserved simple-mode position is never included.
func (o *Orchestrator) Positions() []Position {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.positionsSnapshotLocked()
}

// Scenes returns a snapshot of all known scenes, sorted by ID.
func (o *Orchestrator) Scenes() []Scene {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.scenesSnapshotLocked()
}

// LiveState returns the current preview/on-air selection.
func (o *Orchestrator) LiveState() LiveState {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.live
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
		if p.ID == simplePositionID {
			continue
		}
		ps = append(ps, *p)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].ID < ps[j].ID })
	return ps
}

func (o *Orchestrator) scenesSnapshotLocked() []Scene {
	out := make([]Scene, 0, len(o.scenes))
	for _, s := range o.scenes {
		cp := *s
		cp.PositionIDs = append([]string(nil), s.PositionIDs...)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (o *Orchestrator) statusLocked() SystemStatus {
	streaming := false
	for _, p := range o.positions {
		if p.ID == simplePositionID {
			continue
		}
		if p.CameraID != "" {
			streaming = true
			break
		}
	}
	if !streaming && o.live.LiveKind != LiveKindNone {
		streaming = true
	}
	return SystemStatus{
		ObsConnected:         o.obsCtl.IsConnected(),
		MediaServerConnected: o.mediaConnected,
		Streaming:            streaming,
		ActiveSceneName:      o.programScene,
	}
}

// persistPositionsLocked saves only public positions (ID/Name). Must hold o.mu.
func (o *Orchestrator) persistPositionsLocked() error {
	defs := make([]positions.Position, 0, len(o.positions))
	for _, p := range o.positions {
		if p.ID == simplePositionID {
			continue
		}
		defs = append(defs, positions.Position{ID: p.ID, Name: p.Name})
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	return o.positionsStore.Save(defs)
}

func (o *Orchestrator) persistScenesLocked() error {
	defs := make([]scenes.Scene, 0, len(o.scenes))
	for _, s := range o.scenes {
		ids := append([]string(nil), s.PositionIDs...)
		if ids == nil {
			ids = []string{}
		}
		defs = append(defs, scenes.Scene{ID: s.ID, Name: s.Name, PositionIDs: ids})
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	return o.scenesStore.Save(defs)
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

func (o *Orchestrator) publishScenes() {
	o.hub.Publish(events.Event{Type: "scenes.updated", Payload: o.Scenes()})
}

func (o *Orchestrator) publishLiveState() {
	o.hub.Publish(events.Event{Type: "live.updated", Payload: o.LiveState()})
}

func (o *Orchestrator) publishError(message string) {
	o.hub.Publish(events.Event{Type: "error", Payload: map[string]string{"message": message}})
}

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

func validateSceneName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ErrSceneNameRequired
	}
	return trimmed, nil
}

func dedupePositionIDs(ids []string) []string {
	if len(ids) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (o *Orchestrator) validateScenePositionIDsLocked(ids []string) error {
	for _, id := range ids {
		if id == simplePositionID {
			return ErrReservedPosition
		}
		if _, ok := o.positions[id]; !ok {
			return ErrPositionNotFound
		}
	}
	return nil
}

// ensureHiddenPosition creates the reserved simple-mode position and OBS input once.
func (o *Orchestrator) ensureHiddenPosition() error {
	o.mu.Lock()
	if o.hiddenReady {
		if _, ok := o.positions[simplePositionID]; ok {
			o.mu.Unlock()
			return nil
		}
	}
	o.mu.Unlock()

	inputName := positionInputName(simplePositionID)
	if err := o.obsCtl.CreatePositionInput(o.programScene, inputName); err != nil {
		// Tolerate already-exists (restart / double ensure); still register in memory.
		slog.Debug("create hidden position input", "error", err)
	}

	o.mu.Lock()
	if _, ok := o.positions[simplePositionID]; !ok {
		o.positions[simplePositionID] = &Position{ID: simplePositionID, Name: simplePositionID}
	}
	o.hiddenReady = true
	o.mu.Unlock()
	return nil
}

type offlineUnassignment struct {
	positionID string
	inputName  string
	wasAudio   bool
}

type pendingActions struct {
	removeIDs       []string
	offlineUnassign []offlineUnassignment
}

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

// CreatePosition registers a new named position.
func (o *Orchestrator) CreatePosition(name string) (Position, error) {
	trimmed, err := validatePositionName(name)
	if err != nil {
		return Position{}, err
	}

	o.mu.RLock()
	for _, p := range o.positions {
		if p.ID == simplePositionID {
			continue
		}
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
		if p.ID == simplePositionID {
			continue
		}
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

// RenamePosition changes an existing position's name.
func (o *Orchestrator) RenamePosition(id, newName string) (Position, error) {
	if id == simplePositionID {
		return Position{}, ErrReservedPosition
	}
	trimmed, err := validatePositionName(newName)
	if err != nil {
		return Position{}, err
	}

	o.mu.Lock()
	pos, ok := o.positions[id]
	if !ok || id == simplePositionID {
		o.mu.Unlock()
		return Position{}, ErrPositionNotFound
	}
	changed := pos.Name != trimmed
	if changed {
		for otherID, p := range o.positions {
			if otherID == simplePositionID {
				continue
			}
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

// DeletePosition removes a position.
func (o *Orchestrator) DeletePosition(id string) error {
	if id == simplePositionID {
		return ErrReservedPosition
	}
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

// AssignCamera assigns cameraID to positionID. Visibility is not changed;
// only the OBS source URL is updated. Cut controls on-air visibility.
func (o *Orchestrator) AssignCamera(positionID, cameraID string) (Position, error) {
	if positionID == simplePositionID {
		return Position{}, ErrReservedPosition
	}
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

// UnassignPosition clears positionID's camera assignment.
func (o *Orchestrator) UnassignPosition(positionID string) (Position, error) {
	if positionID == simplePositionID {
		return Position{}, ErrReservedPosition
	}
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

// SetAudioPosition marks positionID as the sole audio source.
func (o *Orchestrator) SetAudioPosition(positionID string) (Position, error) {
	if positionID == simplePositionID {
		return Position{}, ErrReservedPosition
	}
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

// CreateScene creates a named scene referencing existing public positions.
func (o *Orchestrator) CreateScene(name string, positionIDs []string) (Scene, error) {
	trimmed, err := validateSceneName(name)
	if err != nil {
		return Scene{}, err
	}
	ids := dedupePositionIDs(positionIDs)

	o.mu.Lock()
	for _, s := range o.scenes {
		if s.Name == trimmed {
			o.mu.Unlock()
			return Scene{}, ErrSceneNameTaken
		}
	}
	if err := o.validateScenePositionIDsLocked(ids); err != nil {
		o.mu.Unlock()
		return Scene{}, err
	}
	id := scenes.NewID()
	sc := &Scene{ID: id, Name: trimmed, PositionIDs: ids}
	o.scenes[id] = sc
	if err := o.persistScenesLocked(); err != nil {
		delete(o.scenes, id)
		o.mu.Unlock()
		return Scene{}, err
	}
	result := *sc
	result.PositionIDs = append([]string(nil), sc.PositionIDs...)
	o.mu.Unlock()

	o.publishScenes()
	return result, nil
}

// RenameScene renames a scene. Renaming to the current name is a no-op success.
func (o *Orchestrator) RenameScene(id, newName string) (Scene, error) {
	trimmed, err := validateSceneName(newName)
	if err != nil {
		return Scene{}, err
	}

	o.mu.Lock()
	sc, ok := o.scenes[id]
	if !ok {
		o.mu.Unlock()
		return Scene{}, ErrSceneNotFound
	}
	changed := sc.Name != trimmed
	if changed {
		for otherID, s := range o.scenes {
			if otherID != id && s.Name == trimmed {
				o.mu.Unlock()
				return Scene{}, ErrSceneNameTaken
			}
		}
		sc.Name = trimmed
		if err := o.persistScenesLocked(); err != nil {
			o.mu.Unlock()
			return Scene{}, err
		}
	}
	result := *sc
	result.PositionIDs = append([]string(nil), sc.PositionIDs...)
	o.mu.Unlock()

	if changed {
		o.publishScenes()
	}
	return result, nil
}

// UpdateScenePositions replaces the position set of a scene.
// If the scene is currently live, OBS visibility is updated immediately.
func (o *Orchestrator) UpdateScenePositions(id string, positionIDs []string) (Scene, error) {
	ids := dedupePositionIDs(positionIDs)

	o.mu.Lock()
	sc, ok := o.scenes[id]
	if !ok {
		o.mu.Unlock()
		return Scene{}, ErrSceneNotFound
	}
	if err := o.validateScenePositionIDsLocked(ids); err != nil {
		o.mu.Unlock()
		return Scene{}, err
	}
	sc.PositionIDs = ids
	if err := o.persistScenesLocked(); err != nil {
		o.mu.Unlock()
		return Scene{}, err
	}
	isLive := o.live.LiveKind == LiveKindScene && o.live.LiveID == id
	enableIDs := append([]string(nil), ids...)
	result := *sc
	result.PositionIDs = append([]string(nil), sc.PositionIDs...)
	o.mu.Unlock()

	if isLive {
		if err := o.applyExclusiveVisibility(enableIDs); err != nil {
			slog.Warn("failed to apply live scene position update to OBS", "scene", id, "error", err)
		}
	}

	o.publishScenes()
	return result, nil
}

// DeleteScene removes a scene. The live scene cannot be deleted.
// Deleting a scene that is only in preview clears the preview.
func (o *Orchestrator) DeleteScene(id string) error {
	o.mu.Lock()
	if _, ok := o.scenes[id]; !ok {
		o.mu.Unlock()
		return ErrSceneNotFound
	}
	if o.live.LiveKind == LiveKindScene && o.live.LiveID == id {
		o.mu.Unlock()
		return ErrSceneIsLive
	}
	delete(o.scenes, id)
	previewCleared := false
	if o.live.PreviewKind == LiveKindScene && o.live.PreviewID == id {
		o.live.PreviewKind = LiveKindNone
		o.live.PreviewID = ""
		previewCleared = true
	}
	if err := o.persistScenesLocked(); err != nil {
		o.mu.Unlock()
		return err
	}
	o.mu.Unlock()

	o.publishScenes()
	if previewCleared {
		o.publishLiveState()
	}
	return nil
}

// SetPreviewCamera selects a camera for preview (modo simples).
func (o *Orchestrator) SetPreviewCamera(cameraID string) (LiveState, error) {
	o.mu.Lock()
	if _, ok := o.cameras[cameraID]; !ok {
		o.mu.Unlock()
		return LiveState{}, ErrCameraNotFound
	}
	o.live.PreviewKind = LiveKindCamera
	o.live.PreviewID = cameraID
	result := o.live
	o.mu.Unlock()

	o.publishLiveState()
	return result, nil
}

// SetPreviewScene selects a scene for preview.
func (o *Orchestrator) SetPreviewScene(sceneID string) (LiveState, error) {
	o.mu.Lock()
	if _, ok := o.scenes[sceneID]; !ok {
		o.mu.Unlock()
		return LiveState{}, ErrSceneNotFound
	}
	o.live.PreviewKind = LiveKindScene
	o.live.PreviewID = sceneID
	result := o.live
	o.mu.Unlock()

	o.publishLiveState()
	return result, nil
}

// Cut applies the current preview to air, enforcing exclusive position visibility.
func (o *Orchestrator) Cut(ctx context.Context) (LiveState, error) {
	_ = ctx
	if err := o.ensureHiddenPosition(); err != nil {
		return LiveState{}, fmt.Errorf("%w: %v", ErrOBSUnreachable, err)
	}

	o.mu.Lock()
	preview := o.live
	if preview.PreviewKind == LiveKindNone || preview.PreviewID == "" {
		o.mu.Unlock()
		return LiveState{}, ErrPreviewEmpty
	}

	// Idempotent: already live with same selection.
	if preview.LiveKind == preview.PreviewKind && preview.LiveID == preview.PreviewID {
		result := o.live
		o.mu.Unlock()
		return result, nil
	}

	switch preview.PreviewKind {
	case LiveKindCamera:
		cam, ok := o.cameras[preview.PreviewID]
		if !ok {
			o.mu.Unlock()
			return LiveState{}, ErrCameraNotFound
		}
		if cam.Status != StatusOnline {
			o.mu.Unlock()
			return LiveState{}, ErrSourceUnavailable
		}
		sourceURL := cam.SourceURL
		cameraID := cam.ID
		o.mu.Unlock()

		simpleInput := positionInputName(simplePositionID)
		if err := o.obsCtl.UpdatePositionSource(simpleInput, sourceURL); err != nil {
			if errors.Is(err, obs.ErrInputNotFound) {
				return LiveState{}, ErrPositionOBSInputMissing
			}
			return LiveState{}, fmt.Errorf("%w: %v", ErrOBSUnreachable, err)
		}
		if err := o.applyExclusiveVisibility([]string{simplePositionID}); err != nil {
			return LiveState{}, fmt.Errorf("%w: %v", ErrOBSUnreachable, err)
		}
		if err := o.setAudioPositionInternal(simplePositionID); err != nil {
			slog.Warn("failed to set automatic audio for modo simples", "error", err)
		}

		o.mu.Lock()
		if p, ok := o.positions[simplePositionID]; ok {
			// Clear camera from any other position that held it.
			for otherID, op := range o.positions {
				if otherID != simplePositionID && op.CameraID == cameraID {
					op.CameraID = ""
				}
			}
			p.CameraID = cameraID
		}
		o.live.LiveKind = LiveKindCamera
		o.live.LiveID = cameraID
		result := o.live
		o.mu.Unlock()

		o.publishPositions()
		o.publishLiveState()
		o.publishStatus()
		return result, nil

	case LiveKindScene:
		sc, ok := o.scenes[preview.PreviewID]
		if !ok {
			o.mu.Unlock()
			return LiveState{}, ErrSceneNotFound
		}
		// Collect valid position IDs and check assigned cameras for offline.
		enableIDs := make([]string, 0, len(sc.PositionIDs))
		for _, pid := range sc.PositionIDs {
			p, exists := o.positions[pid]
			if !exists || pid == simplePositionID {
				continue
			}
			if p.CameraID != "" {
				cam, camOK := o.cameras[p.CameraID]
				if camOK && cam.Status != StatusOnline {
					o.mu.Unlock()
					return LiveState{}, ErrSourceUnavailable
				}
			}
			enableIDs = append(enableIDs, pid)
		}
		sceneID := sc.ID
		o.mu.Unlock()

		if err := o.applyExclusiveVisibility(enableIDs); err != nil {
			return LiveState{}, fmt.Errorf("%w: %v", ErrOBSUnreachable, err)
		}

		o.mu.Lock()
		o.live.LiveKind = LiveKindScene
		o.live.LiveID = sceneID
		result := o.live
		o.mu.Unlock()

		o.publishLiveState()
		o.publishStatus()
		return result, nil

	default:
		o.mu.Unlock()
		return LiveState{}, ErrPreviewEmpty
	}
}

// applyExclusiveVisibility enables exactly the given position IDs and disables all others.
func (o *Orchestrator) applyExclusiveVisibility(enableIDs []string) error {
	enableSet := make(map[string]struct{}, len(enableIDs))
	for _, id := range enableIDs {
		enableSet[id] = struct{}{}
	}

	o.mu.RLock()
	allIDs := make([]string, 0, len(o.positions))
	for id := range o.positions {
		allIDs = append(allIDs, id)
	}
	o.mu.RUnlock()

	for _, id := range allIDs {
		input := positionInputName(id)
		_, shouldEnable := enableSet[id]
		if err := o.obsCtl.SetPositionEnabled(o.programScene, input, shouldEnable); err != nil {
			return err
		}
	}
	return nil
}

// setAudioPositionInternal sets audio without reserved-position rejection (modo simples).
func (o *Orchestrator) setAudioPositionInternal(positionID string) error {
	o.mu.RLock()
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
		return err
	}

	o.mu.Lock()
	if prevAudioID != "" && prevAudioID != positionID {
		if prevPos, exists := o.positions[prevAudioID]; exists {
			prevPos.IsAudioSource = false
		}
	}
	if pos, ok := o.positions[positionID]; ok {
		pos.IsAudioSource = true
	}
	o.audioPositionID = positionID
	o.mu.Unlock()
	return nil
}
