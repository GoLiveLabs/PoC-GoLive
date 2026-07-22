package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/mediaserver"
	"live-orchestrator/backend/internal/obs/obsmock"
	"live-orchestrator/backend/internal/positions"
	"live-orchestrator/backend/internal/scenes"
)

// fakeMediaClient lets tests script the sequence of ListActiveStreams results.
type fakeMediaClient struct {
	mu      sync.Mutex
	streams []mediaserver.StreamInfo
	err     error
}

func (f *fakeMediaClient) set(streams []mediaserver.StreamInfo, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streams = streams
	f.err = err
}

func (f *fakeMediaClient) ListActiveStreams(ctx context.Context) ([]mediaserver.StreamInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make([]mediaserver.StreamInfo, len(f.streams))
	copy(out, f.streams)
	return out, nil
}

// fakePositionsStore is an in-memory fake of positions.Store, mirroring
// obsmock's style: a struct backed by a slice, no interface-mocking
// framework.
type fakePositionsStore struct {
	mu       sync.Mutex
	loadData []positions.Position
	loadErr  error
	saved    []positions.Position
	saveErr  error
	saveN    int
}

func (f *fakePositionsStore) Load() ([]positions.Position, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	out := make([]positions.Position, len(f.loadData))
	copy(out, f.loadData)
	return out, nil
}

func (f *fakePositionsStore) Save(ps []positions.Position) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveN++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = make([]positions.Position, len(ps))
	copy(f.saved, ps)
	return nil
}

type fakeScenesStore struct {
	mu       sync.Mutex
	loadData []scenes.Scene
	loadErr  error
	saved    []scenes.Scene
	saveErr  error
	saveN    int
}

func (f *fakeScenesStore) Load() ([]scenes.Scene, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	out := make([]scenes.Scene, len(f.loadData))
	copy(out, f.loadData)
	return out, nil
}

func (f *fakeScenesStore) Save(ss []scenes.Scene) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveN++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = make([]scenes.Scene, len(ss))
	copy(f.saved, ss)
	return nil
}

func newTestOrchestrator() (*Orchestrator, *fakeMediaClient, *obsmock.Mock) {
	o, media, obsCtl, _, _ := newTestOrchestratorWithStore()
	return o, media, obsCtl
}

func newTestOrchestratorWithStore() (*Orchestrator, *fakeMediaClient, *obsmock.Mock, *fakePositionsStore, *fakeScenesStore) {
	return newTestOrchestratorWithMediaSourceURL("rtmp://localhost:1935")
}

func newTestOrchestratorWithMediaSourceURL(baseURL string) (*Orchestrator, *fakeMediaClient, *obsmock.Mock, *fakePositionsStore, *fakeScenesStore) {
	media := &fakeMediaClient{}
	obsCtl := obsmock.New()
	hub := events.NewHub()
	store := &fakePositionsStore{}
	sstore := &fakeScenesStore{}
	o := New(media, obsCtl, hub, "Program", time.Second, baseURL, store, sstore)
	return o, media, obsCtl, store, sstore
}

// onlineCamera brings a camera with the given id online via SyncOnce.
func onlineCamera(o *Orchestrator, media *fakeMediaClient, id string) {
	media.set([]mediaserver.StreamInfo{{Name: id, Ready: true}}, nil)
	o.SyncOnce(context.Background())
}

func offlineCamera(o *Orchestrator, media *fakeMediaClient) {
	media.set(nil, nil)
	o.SyncOnce(context.Background())
}

func TestSyncOnce_CameraAppears(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}}, nil)

	cams := o.SyncOnce(context.Background())

	if len(cams) != 1 {
		t.Fatalf("expected 1 camera, got %d", len(cams))
	}
	if cams[0].ID != "camera1" || cams[0].Status != StatusOnline {
		t.Fatalf("unexpected camera state: %+v", cams[0])
	}
}

func TestSyncOnce_UsesConfiguredMediaSourceURL(t *testing.T) {
	o, media, _, _, _ := newTestOrchestratorWithMediaSourceURL("rtmp://example.internal:1935")
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}}, nil)

	cams := o.SyncOnce(context.Background())

	if len(cams) != 1 {
		t.Fatalf("expected 1 camera, got %d", len(cams))
	}
	if cams[0].SourceURL != "rtmp://example.internal:1935/camera1" {
		t.Fatalf("expected source URL to use configured media server base URL, got %q", cams[0].SourceURL)
	}
}

func TestSyncOnce_CameraDisappearsAndReturnsBefore60s(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "camera1")

	cams := offlineCameraSnapshot(o, media)
	if cams[0].Status != StatusOffline {
		t.Fatalf("expected camera to be offline, got %+v", cams[0])
	}

	onlineCamera(o, media, "camera1")
	cams = o.Cameras()
	if len(cams) != 1 || cams[0].Status != StatusOnline {
		t.Fatalf("expected camera back online, got %+v", cams)
	}
}

func offlineCameraSnapshot(o *Orchestrator, media *fakeMediaClient) []Camera {
	media.set(nil, nil)
	return o.SyncOnce(context.Background())
}

func TestSyncOnce_CameraOffline60sPlusIsRemoved(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "camera1")
	offlineCamera(o, media)

	o.mu.Lock()
	o.offlineSince["camera1"] = time.Now().Add(-2 * time.Minute)
	o.mu.Unlock()

	cams := o.SyncOnce(context.Background())
	if len(cams) != 0 {
		t.Fatalf("expected camera to be removed, got %+v", cams)
	}
}

// --- Orchestrator.CreatePosition ---

func TestCreatePosition_Happy(t *testing.T) {
	o, _, obsCtl := newTestOrchestrator()

	pos, err := o.CreatePosition("Principal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos.Name != "Principal" || pos.CameraID != "" || pos.ID == "" {
		t.Fatalf("unexpected position: %+v", pos)
	}
	inputName := positionInputName(pos.ID)
	if _, ok := obsCtl.Inputs[inputName]; !ok {
		t.Fatalf("expected obs input %q to be created", inputName)
	}
	if obsCtl.Enabled[inputName] {
		t.Fatalf("expected scene item to be created disabled")
	}
}

func TestCreatePosition_EmptyName(t *testing.T) {
	o, _, obsCtl := newTestOrchestrator()

	_, err := o.CreatePosition("   ")
	if !errors.Is(err, ErrPositionNameEmpty) {
		t.Fatalf("expected ErrPositionNameEmpty, got %v", err)
	}
	if len(obsCtl.Inputs) != 0 {
		t.Fatalf("expected zero obs calls, got %+v", obsCtl.Inputs)
	}
	if len(o.Positions()) != 0 {
		t.Fatalf("expected no positions created")
	}
}

func TestCreatePosition_DuplicateName(t *testing.T) {
	o, _, obsCtl := newTestOrchestrator()
	if _, err := o.CreatePosition("Principal"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	callsBefore := len(obsCtl.Inputs)

	_, err := o.CreatePosition("Principal")
	if !errors.Is(err, ErrPositionNameTaken) {
		t.Fatalf("expected ErrPositionNameTaken, got %v", err)
	}
	callsAfter := len(obsCtl.Inputs)
	if callsAfter != callsBefore {
		t.Fatalf("expected zero additional obs calls on duplicate name")
	}
}

func TestCreatePosition_OBSFailure(t *testing.T) {
	o, _, obsCtl, store, _ := newTestOrchestratorWithStore()
	obsCtl.CreatePositionInputErr = errors.New("obs unreachable")

	_, err := o.CreatePosition("Canto")
	if err == nil {
		t.Fatalf("expected an error")
	}
	for _, p := range o.Positions() {
		if p.Name == "Canto" {
			t.Fatalf("expected Canto not to be present in Positions()")
		}
	}
	if store.saveN != 0 {
		t.Fatalf("expected the store never to be called, got %d Save calls", store.saveN)
	}
}

func TestCreatePosition_NameAtMaxLength(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	name := strings.Repeat("a", 100)

	pos, err := o.CreatePosition(name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos.Name != name {
		t.Fatalf("expected name to be preserved, got %q", pos.Name)
	}
}

func TestCreatePosition_NameOverMaxLength(t *testing.T) {
	o, _, obsCtl := newTestOrchestrator()
	name := strings.Repeat("a", 101)

	_, err := o.CreatePosition(name)
	if !errors.Is(err, ErrPositionNameTooLong) {
		t.Fatalf("expected ErrPositionNameTooLong, got %v", err)
	}
	if len(obsCtl.Inputs) != 0 {
		t.Fatalf("expected no obs call for an over-length name")
	}
}

func TestCreatePosition_ConcurrentDuplicateCreate(t *testing.T) {
	o, _, _ := newTestOrchestrator()

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := o.CreatePosition("Sala")
			results[idx] = err
		}(i)
	}
	wg.Wait()

	successCount, takenCount := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrPositionNameTaken):
			takenCount++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successCount != 1 || takenCount != 1 {
		t.Fatalf("expected exactly one success and one ErrPositionNameTaken, got %d successes and %d taken", successCount, takenCount)
	}

	count := 0
	for _, p := range o.Positions() {
		if p.Name == "Sala" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one position named Sala, got %d", count)
	}
}

// --- Orchestrator.RenamePosition ---

func TestRenamePosition_Happy(t *testing.T) {
	o, _, obsCtl, store, _ := newTestOrchestratorWithStore()
	pos, _ := o.CreatePosition("Principal")
	saveNBefore := store.saveN
	callsBefore := len(obsCtl.Inputs)

	renamed, err := o.RenamePosition(pos.ID, "Novo Nome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if renamed.Name != "Novo Nome" {
		t.Fatalf("expected renamed position, got %+v", renamed)
	}
	if store.saveN <= saveNBefore {
		t.Fatalf("expected Store.Save to be called")
	}
	if len(obsCtl.Inputs) != callsBefore {
		t.Fatalf("expected renaming never to touch obs")
	}
}

func TestRenamePosition_PreservesAssignment(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	assigned, err := o.AssignCamera(pos.ID, "camera1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	renamed, err := o.RenamePosition(pos.ID, "Novo Nome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if renamed.CameraID != assigned.CameraID {
		t.Fatalf("expected CameraID unchanged after rename, got %+v", renamed)
	}
}

func TestRenamePosition_UnknownID(t *testing.T) {
	o, _, _ := newTestOrchestrator()

	_, err := o.RenamePosition("unknown-id", "X")
	if !errors.Is(err, ErrPositionNotFound) {
		t.Fatalf("expected ErrPositionNotFound, got %v", err)
	}
}

func TestRenamePosition_DuplicateName(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	_, _ = o.CreatePosition("Principal")
	other, _ := o.CreatePosition("Secundária")

	_, err := o.RenamePosition(other.ID, "Principal")
	if !errors.Is(err, ErrPositionNameTaken) {
		t.Fatalf("expected ErrPositionNameTaken, got %v", err)
	}
}

func TestRenamePosition_SameName_Idempotent(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")

	renamed, err := o.RenamePosition(pos.ID, "Principal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if renamed.Name != "Principal" {
		t.Fatalf("unexpected name: %+v", renamed)
	}
}

// --- Orchestrator.DeletePosition ---

func TestDeletePosition_Happy(t *testing.T) {
	o, _, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	inputName := positionInputName(pos.ID)

	if err := o.DeletePosition(pos.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(o.Positions()) != 0 {
		t.Fatalf("expected no positions remaining")
	}
	if _, ok := obsCtl.Inputs[inputName]; ok {
		t.Fatalf("expected obs input to be removed")
	}
}

func TestDeletePosition_WithCameraAssigned(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	if _, err := o.AssignCamera(pos.ID, "camera1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inputName := positionInputName(pos.ID)

	if err := o.DeletePosition(pos.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obsCtl.Enabled[inputName] {
		t.Fatalf("expected position to be disabled before removal")
	}
}

func TestDeletePosition_AudioSource(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(pos.ID, "camera1")
	if _, err := o.SetAudioPosition(pos.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := o.DeletePosition(pos.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range o.Positions() {
		if p.IsAudioSource {
			t.Fatalf("expected no position to be the audio source, got %+v", p)
		}
	}
}

func TestDeletePosition_UnknownAndDoubleDelete(t *testing.T) {
	o, _, _ := newTestOrchestrator()

	if err := o.DeletePosition("unknown-id"); !errors.Is(err, ErrPositionNotFound) {
		t.Fatalf("expected ErrPositionNotFound, got %v", err)
	}

	pos, _ := o.CreatePosition("Principal")
	if err := o.DeletePosition(pos.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := o.DeletePosition(pos.ID); !errors.Is(err, ErrPositionNotFound) {
		t.Fatalf("expected ErrPositionNotFound on double delete, got %v", err)
	}
}

func TestDeletePosition_LastRemainingPosition(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")

	if err := o.DeletePosition(pos.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(o.Positions()) != 0 {
		t.Fatalf("expected empty position list")
	}
	if _, err := o.CreatePosition("Principal"); err != nil {
		t.Fatalf("expected CreatePosition to still succeed: %v", err)
	}
}

// --- Orchestrator.AssignCamera ---

func TestAssignCamera_Happy(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")

	result, err := o.AssignCamera(pos.ID, "camera1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CameraID != "camera1" {
		t.Fatalf("expected camera1 assigned, got %+v", result)
	}
	inputName := positionInputName(pos.ID)
	if obsCtl.Enabled[inputName] {
		t.Fatalf("expected position input still disabled until Cut")
	}
	if !strings.Contains(obsCtl.Inputs[inputName], "camera1") {
		t.Fatalf("expected input source to reference camera1, got %q", obsCtl.Inputs[inputName])
	}
}

func TestAssignCamera_UnknownCamera(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")

	_, err := o.AssignCamera(pos.ID, "unknown-cam")
	if !errors.Is(err, ErrCameraNotFound) {
		t.Fatalf("expected ErrCameraNotFound, got %v", err)
	}
}

func TestAssignCamera_OfflineCamera(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	offlineCamera(o, media)

	_, err := o.AssignCamera(pos.ID, "camera1")
	if !errors.Is(err, ErrCameraOffline) {
		t.Fatalf("expected ErrCameraOffline, got %v", err)
	}
}

func TestAssignCamera_UnknownPosition(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "camera1")

	_, err := o.AssignCamera("unknown-pos", "camera1")
	if !errors.Is(err, ErrPositionNotFound) {
		t.Fatalf("expected ErrPositionNotFound, got %v", err)
	}
}

func TestAssignCamera_OBSInputMissing(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")

	// Simulate the position's OBS input having been removed/renamed outside the panel.
	delete(obsCtl.Inputs, positionInputName(pos.ID))

	result, err := o.AssignCamera(pos.ID, "camera1")
	if !errors.Is(err, ErrPositionOBSInputMissing) {
		t.Fatalf("expected ErrPositionOBSInputMissing, got %v", err)
	}
	if result.CameraID != "" {
		t.Fatalf("expected no commit on failure, got %+v", result)
	}
	for _, p := range o.Positions() {
		if p.ID == pos.ID && p.CameraID != "" {
			t.Fatalf("expected assignment not to be committed, got %+v", p)
		}
	}
}

func TestAssignCamera_BumpsPreviousPosition(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	onlineCamera(o, media, "camera1")
	if _, err := o.AssignCamera(posA.ID, "camera1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := o.AssignCamera(posB.ID, "camera1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var gotA, gotB Position
	for _, p := range o.Positions() {
		if p.ID == posA.ID {
			gotA = p
		}
		if p.ID == posB.ID {
			gotB = p
		}
	}
	if gotA.CameraID != "" {
		t.Fatalf("expected posA to be emptied, got %+v", gotA)
	}
	if gotB.CameraID != "camera1" {
		t.Fatalf("expected posB to hold camera1, got %+v", gotB)
	}
	if obsCtl.Enabled[positionInputName(posA.ID)] {
		t.Fatalf("expected posA's input disabled after bump")
	}
}

func TestAssignCamera_IdempotentSamePosition(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	if _, err := o.AssignCamera(pos.ID, "camera1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inputName := positionInputName(pos.ID)
	urlBefore := obsCtl.Inputs[inputName]

	result, err := o.AssignCamera(pos.ID, "camera1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CameraID != "camera1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	urlAfter := obsCtl.Inputs[inputName]
	if urlBefore != urlAfter {
		t.Fatalf("expected no additional obs mutation on idempotent reassignment")
	}
}

func TestAssignCamera_ConcurrentRace(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	onlineCamera(o, media, "camera1")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = o.AssignCamera(posA.ID, "camera1")
	}()
	go func() {
		defer wg.Done()
		_, _ = o.AssignCamera(posB.ID, "camera1")
	}()
	wg.Wait()

	occupiedCount := 0
	for _, p := range o.Positions() {
		if p.CameraID == "camera1" {
			occupiedCount++
		}
	}
	if occupiedCount != 1 {
		t.Fatalf("expected camera1 to occupy exactly one position, got %d", occupiedCount)
	}
}

func TestAssignCamera_BumpClearsAudio(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(posA.ID, "camera1")
	if _, err := o.SetAudioPosition(posA.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := o.AssignCamera(posB.ID, "camera1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range o.Positions() {
		if p.IsAudioSource {
			t.Fatalf("expected no position to be the audio source, got %+v", p)
		}
	}
	if !obsCtl.Muted[positionInputName(posA.ID)] {
		t.Fatalf("expected posA's input to be muted")
	}
}

// --- Orchestrator.UnassignPosition ---

func TestUnassignPosition_Happy(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(pos.ID, "camera1")

	result, err := o.UnassignPosition(pos.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CameraID != "" {
		t.Fatalf("expected empty CameraID, got %+v", result)
	}
	if obsCtl.Enabled[positionInputName(pos.ID)] {
		t.Fatalf("expected position disabled")
	}
}

func TestUnassignPosition_Idempotent(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")

	result, err := o.UnassignPosition(pos.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CameraID != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUnassignPosition_UnknownID(t *testing.T) {
	o, _, _ := newTestOrchestrator()

	_, err := o.UnassignPosition("unknown-id")
	if !errors.Is(err, ErrPositionNotFound) {
		t.Fatalf("expected ErrPositionNotFound, got %v", err)
	}
}

func TestUnassignPosition_ClearsAudio(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(pos.ID, "camera1")
	_, _ = o.SetAudioPosition(pos.ID)

	result, err := o.UnassignPosition(pos.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsAudioSource {
		t.Fatalf("expected IsAudioSource cleared, got %+v", result)
	}
	if !obsCtl.Muted[positionInputName(pos.ID)] {
		t.Fatalf("expected input muted")
	}
}

// --- Orchestrator.SetAudioPosition ---

func TestSetAudioPosition_Happy(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(pos.ID, "camera1")

	result, err := o.SetAudioPosition(pos.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsAudioSource {
		t.Fatalf("expected IsAudioSource true, got %+v", result)
	}
	if obsCtl.Muted[positionInputName(pos.ID)] {
		t.Fatalf("expected input unmuted")
	}
}

func TestSetAudioPosition_SwitchMutesPrevious(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	onlineCamera(o, media, "camera1")
	onlineCamera(o, media, "camera2")
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}, {Name: "camera2", Ready: true}}, nil)
	o.SyncOnce(context.Background())
	_, _ = o.AssignCamera(posA.ID, "camera1")
	_, _ = o.AssignCamera(posB.ID, "camera2")
	_, _ = o.SetAudioPosition(posA.ID)

	_, err := o.SetAudioPosition(posB.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !obsCtl.Muted[positionInputName(posA.ID)] {
		t.Fatalf("expected posA muted")
	}
	if obsCtl.Muted[positionInputName(posB.ID)] {
		t.Fatalf("expected posB unmuted")
	}
	var gotA, gotB Position
	for _, p := range o.Positions() {
		if p.ID == posA.ID {
			gotA = p
		}
		if p.ID == posB.ID {
			gotB = p
		}
	}
	if gotA.IsAudioSource {
		t.Fatalf("expected posA not audio source, got %+v", gotA)
	}
	if !gotB.IsAudioSource {
		t.Fatalf("expected posB audio source, got %+v", gotB)
	}
}

func TestSetAudioPosition_EmptyPositionRejected(t *testing.T) {
	o, _, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	callsBefore := len(obsCtl.Muted)

	_, err := o.SetAudioPosition(pos.ID)
	if !errors.Is(err, ErrPositionEmpty) {
		t.Fatalf("expected ErrPositionEmpty, got %v", err)
	}
	if len(obsCtl.Muted) != callsBefore {
		t.Fatalf("expected no obs call")
	}
}

func TestSetAudioPosition_UnknownID(t *testing.T) {
	o, _, _ := newTestOrchestrator()

	_, err := o.SetAudioPosition("unknown-id")
	if !errors.Is(err, ErrPositionNotFound) {
		t.Fatalf("expected ErrPositionNotFound, got %v", err)
	}
}

func TestSetAudioPosition_DefaultNoneSelected(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(pos.ID, "camera1")

	for _, p := range o.Positions() {
		if p.IsAudioSource {
			t.Fatalf("expected no position to default to audio source, got %+v", p)
		}
	}
}

// --- Offline auto-unassign hook (SyncOnce) ---

func TestOfflineHook_ClearsPosition(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(pos.ID, "camera1")

	offlineCamera(o, media)

	var got Position
	for _, p := range o.Positions() {
		if p.ID == pos.ID {
			got = p
		}
	}
	if got.CameraID != "" {
		t.Fatalf("expected position cleared, got %+v", got)
	}
	if obsCtl.Enabled[positionInputName(pos.ID)] {
		t.Fatalf("expected position disabled")
	}
}

func TestOfflineHook_ClearsAudio(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(pos.ID, "camera1")
	_, _ = o.SetAudioPosition(pos.ID)

	offlineCamera(o, media)

	for _, p := range o.Positions() {
		if p.IsAudioSource {
			t.Fatalf("expected no audio source, got %+v", p)
		}
	}
	if !obsCtl.Muted[positionInputName(pos.ID)] {
		t.Fatalf("expected input muted")
	}
}

func TestOfflineHook_NoopForUnassignedCamera(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")

	callsBefore := len(obsCtl.Enabled)
	offlineCamera(o, media)

	var got Position
	for _, p := range o.Positions() {
		if p.ID == pos.ID {
			got = p
		}
	}
	if got.CameraID != "" {
		t.Fatalf("expected position to remain unassigned, got %+v", got)
	}
	if len(obsCtl.Enabled) != callsBefore {
		t.Fatalf("expected no position-related obs calls")
	}
}

func TestOfflineHook_NoAutoReattachOnReturn(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(pos.ID, "camera1")
	offlineCamera(o, media)

	onlineCamera(o, media, "camera1")

	for _, p := range o.Positions() {
		if p.CameraID != "" {
			t.Fatalf("expected camera not to be automatically reattached, got %+v", p)
		}
	}
}

func TestOfflineHook_TwoSimultaneousOfflineTransitions(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}, {Name: "camera2", Ready: true}}, nil)
	o.SyncOnce(context.Background())
	_, _ = o.AssignCamera(posA.ID, "camera1")
	_, _ = o.AssignCamera(posB.ID, "camera2")
	_, _ = o.SetAudioPosition(posA.ID)

	offlineCamera(o, media)

	var gotA, gotB Position
	for _, p := range o.Positions() {
		if p.ID == posA.ID {
			gotA = p
		}
		if p.ID == posB.ID {
			gotB = p
		}
	}
	if gotA.CameraID != "" || gotB.CameraID != "" {
		t.Fatalf("expected both positions cleared, got %+v and %+v", gotA, gotB)
	}
	if gotA.IsAudioSource {
		t.Fatalf("expected posA no longer the audio source")
	}
	if gotB.IsAudioSource {
		t.Fatalf("expected posB was never the audio source")
	}
	if !obsCtl.Muted[positionInputName(posA.ID)] {
		t.Fatalf("expected posA's input muted")
	}
	if obsCtl.Muted[positionInputName(posB.ID)] {
		t.Fatalf("expected posB's input untouched by mute (was never audio source)")
	}
}

// --- Scale and independence ---

func TestScale_500Positions(t *testing.T) {
	o, _, _ := newTestOrchestrator()

	for i := range 500 {
		if _, err := o.CreatePosition(fmt.Sprintf("Posição %d", i)); err != nil {
			t.Fatalf("CreatePosition #%d failed: %v", i, err)
		}
	}
	if len(o.Positions()) != 500 {
		t.Fatalf("expected 500 positions, got %d", len(o.Positions()))
	}
}

func TestScale_IndependentAssignments(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	var pos [4]Position
	for i := range pos {
		p, err := o.CreatePosition(strings.Repeat("P", i+1))
		if err != nil {
			t.Fatalf("CreatePosition: %v", err)
		}
		pos[i] = p
	}
	streams := make([]mediaserver.StreamInfo, 4)
	for i := range streams {
		streams[i] = mediaserver.StreamInfo{Name: strings.Repeat("c", i+1), Ready: true}
	}
	media.set(streams, nil)
	o.SyncOnce(context.Background())

	for i := range pos {
		if _, err := o.AssignCamera(pos[i].ID, streams[i].Name); err != nil {
			t.Fatalf("AssignCamera #%d: %v", i, err)
		}
	}

	byID := make(map[string]Position)
	for _, p := range o.Positions() {
		byID[p.ID] = p
	}
	for i := range pos {
		if byID[pos[i].ID].CameraID != streams[i].Name {
			t.Fatalf("expected position %d to hold %q, got %+v", i, streams[i].Name, byID[pos[i].ID])
		}
		if obsCtl.Enabled[positionInputName(pos[i].ID)] {
			t.Fatalf("expected position %d still disabled until Cut", i)
		}
	}
	_ = obsCtl
}

func TestScale_ZeroEnabledWithNothingAssigned(t *testing.T) {
	o, _, obsCtl := newTestOrchestrator()
	for i := range 3 {
		if _, err := o.CreatePosition(fmt.Sprintf("Zona %d", i)); err != nil {
			t.Fatalf("CreatePosition: %v", err)
		}
	}

	for _, enabled := range obsCtl.Enabled {
		if enabled {
			t.Fatalf("expected zero enabled scene items, found one enabled")
		}
	}
}

func TestScale_OfflineOnlineOfflineFlapping(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	_, _ = o.AssignCamera(pos.ID, "camera1")

	offlineCamera(o, media)
	onlineCamera(o, media, "camera1")
	offlineCamera(o, media)

	var got Position
	for _, p := range o.Positions() {
		if p.ID == pos.ID {
			got = p
		}
	}
	if got.CameraID != "" {
		t.Fatalf("expected position unassigned after flapping, got %+v", got)
	}
	_ = obsCtl // Enabled state already covered by other assertions; flapping shouldn't re-enable.
}

// --- Integration: persistence across restart (IT-001) ---

func TestIT001_PositionsPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	storePath := dir + "/positions.json"
	store := positions.NewFileStore(storePath)

	media1 := &fakeMediaClient{}
	obsCtl1 := obsmock.New()
	hub1 := events.NewHub()
	sstore := &fakeScenesStore{}
	o1 := New(media1, obsCtl1, hub1, "Program", time.Second, "rtmp://localhost:1935", store, sstore)

	pos, err := o1.CreatePosition("Principal")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	onlineCamera(o1, media1, "camera1")
	if _, err := o1.AssignCamera(pos.ID, "camera1"); err != nil {
		t.Fatalf("AssignCamera: %v", err)
	}

	// Simulate a restart: a second Orchestrator against the same file path.
	store2 := positions.NewFileStore(storePath)
	media2 := &fakeMediaClient{}
	obsCtl2 := obsmock.New()
	hub2 := events.NewHub()
	o2 := New(media2, obsCtl2, hub2, "Program", time.Second, "rtmp://localhost:1935", store2, &fakeScenesStore{})

	got := o2.Positions()
	if len(got) != 1 {
		t.Fatalf("expected 1 position after restart, got %d", len(got))
	}
	if got[0].ID != pos.ID || got[0].Name != "Principal" {
		t.Fatalf("expected same ID/Name preserved, got %+v", got[0])
	}
	if got[0].CameraID != "" {
		t.Fatalf("expected runtime CameraID reset, got %+v", got[0])
	}
	if got[0].IsAudioSource {
		t.Fatalf("expected runtime IsAudioSource reset, got %+v", got[0])
	}
}

// --- Integration: offline auto-unassign publishes positions.updated (IT-005) ---

func TestIT005_OfflineTransitionPublishesPositionsUpdated(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	pos, _ := o.CreatePosition("Principal")
	onlineCamera(o, media, "camera1")
	if _, err := o.AssignCamera(pos.ID, "camera1"); err != nil {
		t.Fatalf("AssignCamera: %v", err)
	}

	ch, cancel := o.hub.Subscribe()
	defer cancel()

	offlineCamera(o, media)

	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type != "positions.updated" {
				continue
			}
			payload, ok := ev.Payload.([]Position)
			if !ok {
				t.Fatalf("unexpected payload type: %T", ev.Payload)
			}
			for _, p := range payload {
				if p.ID == pos.ID {
					if p.CameraID != "" {
						t.Fatalf("expected position cleared in published payload, got %+v", p)
					}
					return
				}
			}
			t.Fatalf("expected published payload to include position %q", pos.ID)
		case <-timeout:
			t.Fatalf("timed out waiting for positions.updated event")
		}
	}
}

// --- Scene CRUD (UT-005�UT-027) ---

func TestCreateScene_Happy(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	sc, err := o.CreateScene("Entrevista", []string{posA.ID, posB.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.Name != "Entrevista" || len(sc.PositionIDs) != 2 {
		t.Fatalf("unexpected scene: %+v", sc)
	}
	found := false
	for _, s := range o.Scenes() {
		if s.ID == sc.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("scene not in Scenes()")
	}
}

func TestCreateScene_EmptyName(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	if _, err := o.CreateScene("", nil); !errors.Is(err, ErrSceneNameRequired) {
		t.Fatalf("expected ErrSceneNameRequired, got %v", err)
	}
}

func TestCreateScene_NameTaken(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	if _, err := o.CreateScene("Entrevista", nil); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := o.CreateScene("Entrevista", nil); !errors.Is(err, ErrSceneNameTaken) {
		t.Fatalf("expected ErrSceneNameTaken, got %v", err)
	}
}

func TestCreateScene_EmptyPositions(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, err := o.CreateScene("Vazia", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sc.PositionIDs) != 0 {
		t.Fatalf("expected zero positions, got %+v", sc)
	}
}

func TestCreateScene_UnknownPosition(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	if _, err := o.CreateScene("X", []string{"unknown-id"}); !errors.Is(err, ErrPositionNotFound) {
		t.Fatalf("expected ErrPositionNotFound, got %v", err)
	}
}

func TestCreateScene_ReservedPosition(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	_ = o.ensureHiddenPosition()
	if _, err := o.CreateScene("X", []string{simplePositionID}); !errors.Is(err, ErrReservedPosition) {
		t.Fatalf("expected ErrReservedPosition, got %v", err)
	}
}

func TestCreateScene_ConcurrentSameName(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := o.CreateScene("Dup", nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	var okN, takenN int
	for err := range errs {
		if err == nil {
			okN++
		} else if errors.Is(err, ErrSceneNameTaken) {
			takenN++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if okN != 1 || takenN != 1 {
		t.Fatalf("expected 1 success and 1 taken, got ok=%d taken=%d", okN, takenN)
	}
	n := 0
	for _, s := range o.Scenes() {
		if s.Name == "Dup" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly one scene named Dup, got %d", n)
	}
}

func TestRenameScene_Happy(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, _ := o.CreateScene("Old", nil)
	got, err := o.RenameScene(sc.ID, "Novo Nome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Novo Nome" {
		t.Fatalf("unexpected name: %+v", got)
	}
}

func TestRenameScene_SameName(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, _ := o.CreateScene("Same", nil)
	if _, err := o.RenameScene(sc.ID, "Same"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenameScene_NameTaken(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	a, _ := o.CreateScene("A", nil)
	_, _ = o.CreateScene("B", nil)
	if _, err := o.RenameScene(a.ID, "B"); !errors.Is(err, ErrSceneNameTaken) {
		t.Fatalf("expected ErrSceneNameTaken, got %v", err)
	}
}

func TestRenameScene_EmptyName(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, _ := o.CreateScene("A", nil)
	if _, err := o.RenameScene(sc.ID, ""); !errors.Is(err, ErrSceneNameRequired) {
		t.Fatalf("expected ErrSceneNameRequired, got %v", err)
	}
}

func TestUpdateScenePositions_Add(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	posC, _ := o.CreatePosition("C")
	onlineCamera(o, media, "cam1")
	_, _ = o.AssignCamera(posA.ID, "cam1")
	sc, _ := o.CreateScene("S", []string{posA.ID, posB.ID})
	_, _ = o.SetPreviewScene(sc.ID)
	if _, err := o.Cut(context.Background()); err != nil {
		t.Fatalf("cut: %v", err)
	}
	got, err := o.UpdateScenePositions(sc.ID, []string{posA.ID, posB.ID, posC.ID})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(got.PositionIDs) != 3 {
		t.Fatalf("expected 3 positions, got %+v", got)
	}
	if !obsCtl.Enabled[positionInputName(posC.ID)] {
		t.Fatalf("expected posC enabled after live update")
	}
}

func TestUpdateScenePositions_Remove(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	sc, _ := o.CreateScene("S", []string{posA.ID, posB.ID})
	got, err := o.UpdateScenePositions(sc.ID, []string{posA.ID})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(got.PositionIDs) != 1 || got.PositionIDs[0] != posA.ID {
		t.Fatalf("unexpected: %+v", got)
	}
	foundB := false
	for _, p := range o.Positions() {
		if p.ID == posB.ID {
			foundB = true
		}
	}
	if !foundB {
		t.Fatalf("posB should still exist as position")
	}
}

func TestUpdateScenePositions_Dedupe(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	sc, _ := o.CreateScene("S", []string{posA.ID})
	got, err := o.UpdateScenePositions(sc.ID, []string{posA.ID, posA.ID})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(got.PositionIDs) != 1 {
		t.Fatalf("expected deduped, got %+v", got)
	}
}

func TestUpdateScenePositions_NotFound(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	if _, err := o.UpdateScenePositions("gone", nil); !errors.Is(err, ErrSceneNotFound) {
		t.Fatalf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestUpdateScenePositions_LiveRemovesDisablesOBS(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	onlineCamera(o, media, "cam1")
	_, _ = o.AssignCamera(posA.ID, "cam1")
	sc, _ := o.CreateScene("S", []string{posA.ID, posB.ID})
	_, _ = o.SetPreviewScene(sc.ID)
	if _, err := o.Cut(context.Background()); err != nil {
		t.Fatalf("cut: %v", err)
	}
	if !obsCtl.Enabled[positionInputName(posB.ID)] {
		t.Fatalf("posB should be enabled while live")
	}
	if _, err := o.UpdateScenePositions(sc.ID, []string{posA.ID}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if obsCtl.Enabled[positionInputName(posB.ID)] {
		t.Fatalf("posB should be disabled after removal from live scene")
	}
}

func TestDeleteScene_Happy(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, _ := o.CreateScene("S", nil)
	if err := o.DeleteScene(sc.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(o.Scenes()) != 0 {
		t.Fatalf("expected empty scenes")
	}
}

func TestDeleteScene_LiveBlocked(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, _ := o.CreateScene("S", nil)
	_, _ = o.SetPreviewScene(sc.ID)
	if _, err := o.Cut(context.Background()); err != nil {
		t.Fatalf("cut: %v", err)
	}
	if err := o.DeleteScene(sc.ID); !errors.Is(err, ErrSceneIsLive) {
		t.Fatalf("expected ErrSceneIsLive, got %v", err)
	}
}

func TestDeleteScene_PreviewOnlyClearsPreview(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, _ := o.CreateScene("S", nil)
	_, _ = o.SetPreviewScene(sc.ID)
	if err := o.DeleteScene(sc.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	ls := o.LiveState()
	if ls.PreviewKind != LiveKindNone || ls.PreviewID != "" {
		t.Fatalf("expected preview cleared, got %+v", ls)
	}
}

func TestDeleteScene_Double(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, _ := o.CreateScene("S", nil)
	_ = o.DeleteScene(sc.ID)
	if err := o.DeleteScene(sc.ID); !errors.Is(err, ErrSceneNotFound) {
		t.Fatalf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestDeleteScene_ConcurrentWithCut(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, _ := o.CreateScene("S", nil)
	_, _ = o.SetPreviewScene(sc.ID)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = o.DeleteScene(sc.ID)
	}()
	go func() {
		defer wg.Done()
		_, _ = o.Cut(context.Background())
	}()
	wg.Wait()
	// Either deleted (not live) or live; must not panic.
	_ = o.LiveState()
	_ = o.Scenes()
}

func TestScenes_List(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	if len(o.Scenes()) != 0 {
		t.Fatalf("expected empty")
	}
	_, _ = o.CreateScene("A", nil)
	_, _ = o.CreateScene("B", nil)
	if len(o.Scenes()) != 2 {
		t.Fatalf("expected 2, got %d", len(o.Scenes()))
	}
}

func TestScene_DynamicCameraResolution(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	streams := []mediaserver.StreamInfo{{Name: "cam1", Ready: true}, {Name: "cam2", Ready: true}}
	media.set(streams, nil)
	o.SyncOnce(context.Background())
	_, _ = o.AssignCamera(posA.ID, "cam1")
	sc, _ := o.CreateScene("Entrevista", []string{posA.ID})
	_, _ = o.AssignCamera(posA.ID, "cam2")
	_, _ = o.SetPreviewScene(sc.ID)
	if _, err := o.Cut(context.Background()); err != nil {
		t.Fatalf("cut: %v", err)
	}
	if !strings.Contains(obsCtl.Inputs[positionInputName(posA.ID)], "cam2") {
		t.Fatalf("expected cam2 source after cut, got %q", obsCtl.Inputs[positionInputName(posA.ID)])
	}
}

func TestScene_MissingPositionSkippedOnCut(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	posX, _ := o.CreatePosition("X")
	onlineCamera(o, media, "cam1")
	_, _ = o.AssignCamera(posX.ID, "cam1")
	sc, _ := o.CreateScene("S", []string{posX.ID})
	if err := o.DeletePosition(posX.ID); err != nil {
		t.Fatalf("delete pos: %v", err)
	}
	found := false
	for _, s := range o.Scenes() {
		if s.ID == sc.ID {
			found = true
			if len(s.PositionIDs) != 1 || s.PositionIDs[0] != posX.ID {
				t.Fatalf("scene should still list posX id, got %+v", s)
			}
		}
	}
	if !found {
		t.Fatalf("scene should still be listed")
	}
	_, _ = o.SetPreviewScene(sc.ID)
	if _, err := o.Cut(context.Background()); err != nil {
		t.Fatalf("cut should succeed skipping missing position: %v", err)
	}
}

// --- Preview and Cut (UT-028�UT-049) ---

func TestSetPreviewCamera_Happy(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "cam1")
	ls, err := o.SetPreviewCamera("cam1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ls.PreviewKind != LiveKindCamera || ls.PreviewID != "cam1" {
		t.Fatalf("unexpected: %+v", ls)
	}
	if ls.LiveKind != LiveKindNone {
		t.Fatalf("live should be unchanged empty: %+v", ls)
	}
}

func TestSetPreviewScene_Happy(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	sc, _ := o.CreateScene("S", nil)
	ls, err := o.SetPreviewScene(sc.ID)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ls.PreviewKind != LiveKindScene || ls.PreviewID != sc.ID {
		t.Fatalf("unexpected: %+v", ls)
	}
}

func TestSetPreviewCamera_MirrorsLive(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "cam1")
	_, _ = o.SetPreviewCamera("cam1")
	_, _ = o.Cut(context.Background())
	ls, err := o.SetPreviewCamera("cam1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ls.PreviewID != "cam1" || ls.LiveID != "cam1" {
		t.Fatalf("unexpected: %+v", ls)
	}
}

func TestSetPreviewCamera_Unknown(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	if _, err := o.SetPreviewCamera("unknown-cam"); !errors.Is(err, ErrCameraNotFound) {
		t.Fatalf("expected ErrCameraNotFound, got %v", err)
	}
}

func TestSetPreviewScene_Unknown(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	if _, err := o.SetPreviewScene("unknown-scene"); !errors.Is(err, ErrSceneNotFound) {
		t.Fatalf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestCut_CameraExclusive(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("A")
	onlineCamera(o, media, "cam1")
	_, _ = o.AssignCamera(pos.ID, "cam1")
	_, _ = o.SetPreviewCamera("cam1")
	ls, err := o.Cut(context.Background())
	if err != nil {
		t.Fatalf("cut: %v", err)
	}
	if ls.LiveKind != LiveKindCamera || ls.LiveID != "cam1" {
		t.Fatalf("unexpected live: %+v", ls)
	}
	simpleIn := positionInputName(simplePositionID)
	if !obsCtl.Enabled[simpleIn] {
		t.Fatalf("hidden position should be enabled")
	}
	if obsCtl.Enabled[positionInputName(pos.ID)] {
		t.Fatalf("named position should be disabled")
	}
}

func TestCut_SceneExclusive(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	posB, _ := o.CreatePosition("B")
	streams := []mediaserver.StreamInfo{{Name: "c1", Ready: true}, {Name: "c2", Ready: true}}
	media.set(streams, nil)
	o.SyncOnce(context.Background())
	_, _ = o.AssignCamera(posA.ID, "c1")
	_, _ = o.AssignCamera(posB.ID, "c2")
	sc, _ := o.CreateScene("S", []string{posA.ID, posB.ID})
	_, _ = o.SetPreviewScene(sc.ID)
	if _, err := o.Cut(context.Background()); err != nil {
		t.Fatalf("cut: %v", err)
	}
	if !obsCtl.Enabled[positionInputName(posA.ID)] || !obsCtl.Enabled[positionInputName(posB.ID)] {
		t.Fatalf("both scene positions should be enabled")
	}
	if obsCtl.Enabled[positionInputName(simplePositionID)] {
		t.Fatalf("hidden should be disabled")
	}
}

func TestCut_EmptyPreview(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	if _, err := o.Cut(context.Background()); !errors.Is(err, ErrPreviewEmpty) {
		t.Fatalf("expected ErrPreviewEmpty, got %v", err)
	}
}

func TestCut_OfflineCamera(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "cam1")
	_, _ = o.SetPreviewCamera("cam1")
	offlineCamera(o, media)
	before := o.LiveState()
	if _, err := o.Cut(context.Background()); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("expected ErrSourceUnavailable, got %v", err)
	}
	after := o.LiveState()
	if after != before {
		t.Fatalf("live state should be retained: before=%+v after=%+v", before, after)
	}
}

func TestCut_Idempotent(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	onlineCamera(o, media, "cam1")
	_, _ = o.SetPreviewCamera("cam1")
	if _, err := o.Cut(context.Background()); err != nil {
		t.Fatalf("first cut: %v", err)
	}
	simpleIn := positionInputName(simplePositionID)
	if !obsCtl.Enabled[simpleIn] {
		t.Fatalf("should be enabled")
	}
	// Force disable then second cut should still be no-op if already live same
	// (implementation returns early without OBS). Second cut must not error.
	ls, err := o.Cut(context.Background())
	if err != nil {
		t.Fatalf("second cut: %v", err)
	}
	if ls.LiveID != "cam1" {
		t.Fatalf("unexpected: %+v", ls)
	}
}

func TestCut_SceneToCamera(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	onlineCamera(o, media, "cam1")
	_, _ = o.AssignCamera(posA.ID, "cam1")
	sc, _ := o.CreateScene("S", []string{posA.ID})
	_, _ = o.SetPreviewScene(sc.ID)
	_, _ = o.Cut(context.Background())
	_, _ = o.SetPreviewCamera("cam1")
	_, _ = o.Cut(context.Background())
	if obsCtl.Enabled[positionInputName(posA.ID)] {
		t.Fatalf("scene pos should be disabled")
	}
	if !obsCtl.Enabled[positionInputName(simplePositionID)] {
		t.Fatalf("hidden should be enabled")
	}
}

func TestCut_CameraToScene(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	posA, _ := o.CreatePosition("A")
	onlineCamera(o, media, "cam1")
	_, _ = o.AssignCamera(posA.ID, "cam1")
	sc, _ := o.CreateScene("S", []string{posA.ID})
	_, _ = o.SetPreviewCamera("cam1")
	_, _ = o.Cut(context.Background())
	_, _ = o.SetPreviewScene(sc.ID)
	_, _ = o.Cut(context.Background())
	if obsCtl.Enabled[positionInputName(simplePositionID)] {
		t.Fatalf("hidden should be disabled")
	}
	if !obsCtl.Enabled[positionInputName(posA.ID)] {
		t.Fatalf("scene pos should be enabled")
	}
}

func TestAssignCamera_DoesNotEnable(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	pos, _ := o.CreatePosition("A")
	onlineCamera(o, media, "cam1")
	if _, err := o.AssignCamera(pos.ID, "cam1"); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if obsCtl.Enabled[positionInputName(pos.ID)] {
		t.Fatalf("should remain disabled until Cut")
	}
}

func TestAssignCamera_Reserved(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "cam1")
	_ = o.ensureHiddenPosition()
	if _, err := o.AssignCamera(simplePositionID, "cam1"); !errors.Is(err, ErrReservedPosition) {
		t.Fatalf("expected ErrReservedPosition, got %v", err)
	}
}

func TestCut_PreviewOfflineAcceptedThenRejected(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "offline-cam")
	_, _ = o.SetPreviewCamera("offline-cam")
	offlineCamera(o, media)
	if _, err := o.Cut(context.Background()); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("expected ErrSourceUnavailable, got %v", err)
	}
}

func TestLive_CameraStaysInLiveStateWhenOffline(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "cam1")
	_, _ = o.SetPreviewCamera("cam1")
	_, _ = o.Cut(context.Background())
	offlineCamera(o, media)
	ls := o.LiveState()
	if ls.LiveID != "cam1" {
		t.Fatalf("live id should remain, got %+v", ls)
	}
	cams := o.Cameras()
	if len(cams) != 1 || cams[0].Status != StatusOffline {
		t.Fatalf("camera should be offline: %+v", cams)
	}
}

func TestSetPreviewCamera_ZeroCameras(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	if len(o.Cameras()) != 0 {
		t.Fatalf("expected zero cameras")
	}
	if _, err := o.SetPreviewCamera("any"); !errors.Is(err, ErrCameraNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestCut_RapidSequenceLastWins(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	streams := []mediaserver.StreamInfo{{Name: "cam1", Ready: true}, {Name: "cam2", Ready: true}}
	media.set(streams, nil)
	o.SyncOnce(context.Background())
	_, _ = o.SetPreviewCamera("cam1")
	_, _ = o.Cut(context.Background())
	_, _ = o.SetPreviewCamera("cam2")
	_, _ = o.Cut(context.Background())
	if o.LiveState().LiveID != "cam2" {
		t.Fatalf("expected cam2 live, got %+v", o.LiveState())
	}
}

func TestCut_SameCameraNoOp(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	onlineCamera(o, media, "cam1")
	_, _ = o.SetPreviewCamera("cam1")
	_, _ = o.Cut(context.Background())
	simpleIn := positionInputName(simplePositionID)
	if !obsCtl.Enabled[simpleIn] {
		t.Fatalf("enabled after first cut")
	}
	_, _ = o.SetPreviewCamera("cam1")
	_, _ = o.Cut(context.Background())
	if !obsCtl.Enabled[simpleIn] {
		t.Fatalf("should stay enabled continuously")
	}
}

func TestCut_ModoSimplesAudioFollows(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "cam1")
	_, _ = o.SetPreviewCamera("cam1")
	if _, err := o.Cut(context.Background()); err != nil {
		t.Fatalf("cut: %v", err)
	}
	o.mu.RLock()
	audioID := o.audioPositionID
	o.mu.RUnlock()
	if audioID != simplePositionID {
		t.Fatalf("expected audio on hidden position, got %q", audioID)
	}
}

func TestCut_ModoSimplesNoAudioError(t *testing.T) {
	// No audio-track detection in system; cut still succeeds for any online camera.
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "cam2")
	_, _ = o.SetPreviewCamera("cam2")
	if _, err := o.Cut(context.Background()); err != nil {
		t.Fatalf("cut should succeed: %v", err)
	}
}

func TestCut_SourceUnavailableRetainsPreview(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	onlineCamera(o, media, "cam1")
	_, _ = o.SetPreviewCamera("cam1")
	// Mark offline without pruning via SyncOnce so preview selection remains.
	o.mu.Lock()
	if cam, ok := o.cameras["cam1"]; ok {
		cam.Status = StatusOffline
	}
	o.mu.Unlock()
	if _, err := o.Cut(context.Background()); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("expected ErrSourceUnavailable, got %v", err)
	}
	ls := o.LiveState()
	if ls.PreviewKind != LiveKindCamera || ls.PreviewID != "cam1" {
		t.Fatalf("preview retained: %+v", ls)
	}
}

func TestHiddenPosition_ExcludedFromPositions(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	_ = o.ensureHiddenPosition()
	for _, p := range o.Positions() {
		if p.ID == simplePositionID {
			t.Fatalf("hidden position must not appear in Positions()")
		}
	}
}
