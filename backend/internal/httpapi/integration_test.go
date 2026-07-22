package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/mediaserver"
	"live-orchestrator/backend/internal/obs/obsmock"
	"live-orchestrator/backend/internal/orchestrator"
	"live-orchestrator/backend/internal/positions"
	"live-orchestrator/backend/internal/scenes"
)

// fakeMediaClient lets integration tests script which streams the media
// server reports as active, so cameras can be brought online for
// AssignCamera to succeed.
type fakeMediaClient struct {
	mu      sync.Mutex
	streams []mediaserver.StreamInfo
}

func (f *fakeMediaClient) set(streams []mediaserver.StreamInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streams = streams
}

func (f *fakeMediaClient) ListActiveStreams(ctx context.Context) ([]mediaserver.StreamInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]mediaserver.StreamInfo, len(f.streams))
	copy(out, f.streams)
	return out, nil
}

// newIntegrationServer wires a real Orchestrator (backed by a real
// positions.FileStore rooted at t.TempDir()) and obsmock.Mock behind a real
// Server, matching _techspec.md's "Backend integration" testing approach.
func newIntegrationServer(t *testing.T) (http.Handler, *orchestrator.Orchestrator, *fakeMediaClient, *obsmock.Mock, string) {
	t.Helper()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "positions.json")
	store := positions.NewFileStore(storePath)
	sstore := scenes.NewFileStore(filepath.Join(dir, "scenes.json"))
	media := &fakeMediaClient{}
	obsCtl := obsmock.New()
	hub := events.NewHub()
	orch := orchestrator.New(media, obsCtl, hub, "Program", time.Second, "rtmp://localhost:1935", store, sstore)

	srv := NewServer(orch, hub, testToken, nil, nil, nil, nil, nil)
	return srv.Handler(), orch, media, obsCtl, storePath
}

// onlineCameras brings every given camera id online simultaneously via a
// single SyncOnce call (a fakeMediaClient.set replaces the whole stream
// list, so cameras must be listed together to all stay online at once).
func onlineCameras(media *fakeMediaClient, orch *orchestrator.Orchestrator, ids ...string) {
	streams := make([]mediaserver.StreamInfo, len(ids))
	for i, id := range ids {
		streams[i] = mediaserver.StreamInfo{Name: id, Ready: true}
	}
	media.set(streams)
	orch.SyncOnce(context.Background())
}

func createPositionHTTP(t *testing.T, srv http.Handler, name string) orchestrator.Position {
	t.Helper()
	rec := doRequest(srv, http.MethodPost, "/api/v1/positions", `{"name":"`+name+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating position %q: expected 201, got %d: %s", name, rec.Code, rec.Body.String())
	}
	return decodePosition(t, rec)
}

// IT-002: create a position, assign an online camera to it, then GET the
// list; expect the assignment to show and obsmock to have enabled only that
// position's input.
func TestIntegration_AssignCamera_FullFlow(t *testing.T) {
	srv, orch, media, obsCtl, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")

	pos := createPositionHTTP(t, srv, "Principal")

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/"+pos.ID+"/camera", `{"cameraId":"cam1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	assigned := decodePosition(t, rec)
	if assigned.CameraID != "cam1" {
		t.Fatalf("expected cameraId cam1, got %+v", assigned)
	}

	rec = doRequest(srv, http.MethodGet, "/api/v1/positions", "")
	var list []orchestrator.Position
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decoding list: %v", err)
	}
	if len(list) != 1 || list[0].CameraID != "cam1" {
		t.Fatalf("unexpected list: %+v", list)
	}

	inputName := "pos_" + pos.ID
	if enabled := obsCtl.Enabled[inputName]; enabled {
		t.Fatalf("expected position still disabled until Cut for %q", inputName)
	}
}

// IT-003: create two positions, assign a camera to the first, then reassign
// the same camera to the second; the first must end up empty.
func TestIntegration_ReassignCamera_BetweenPositions(t *testing.T) {
	srv, orch, media, _, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")

	first := createPositionHTTP(t, srv, "Primeira")
	second := createPositionHTTP(t, srv, "Segunda")

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/"+first.ID+"/camera", `{"cameraId":"cam1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("assigning to first: expected 200, got %d", rec.Code)
	}

	rec = doRequest(srv, http.MethodPost, "/api/v1/positions/"+second.ID+"/camera", `{"cameraId":"cam1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("assigning to second: expected 200, got %d", rec.Code)
	}

	rec = doRequest(srv, http.MethodGet, "/api/v1/positions", "")
	var list []orchestrator.Position
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decoding list: %v", err)
	}
	byID := make(map[string]orchestrator.Position, len(list))
	for _, p := range list {
		byID[p.ID] = p
	}
	if byID[first.ID].CameraID != "" {
		t.Fatalf("expected first position empty, got %+v", byID[first.ID])
	}
	if byID[second.ID].CameraID != "cam1" {
		t.Fatalf("expected second position holding cam1, got %+v", byID[second.ID])
	}
}

// IT-004: create a position, assign a camera, delete it; expect 204, the
// obsmock input removed, and a subsequent assignment attempt on the deleted
// id to 404.
func TestIntegration_DeletePosition_WithCameraAssigned(t *testing.T) {
	srv, orch, media, obsCtl, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")

	pos := createPositionHTTP(t, srv, "Principal")
	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/"+pos.ID+"/camera", `{"cameraId":"cam1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("assigning camera: expected 200, got %d", rec.Code)
	}

	rec = doRequest(srv, http.MethodDelete, "/api/v1/positions/"+pos.ID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}

	inputName := "pos_" + pos.ID
	if _, exists := obsCtl.Inputs[inputName]; exists {
		t.Fatalf("expected obsmock input %q to be removed", inputName)
	}

	rec = doRequest(srv, http.MethodPost, "/api/v1/positions/"+pos.ID+"/camera", `{"cameraId":"cam1"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted position, got %d", rec.Code)
	}
}

// IT-006: assign a distinct camera to each of two positions, mark the first
// as the audio source then the second; expect obsmock to show the first
// muted and the second unmuted, with both positions still holding their
// cameras.
func TestIntegration_AudioExclusivity_EndToEnd(t *testing.T) {
	srv, orch, media, obsCtl, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1", "cam2")

	first := createPositionHTTP(t, srv, "Primeira")
	second := createPositionHTTP(t, srv, "Segunda")

	if rec := doRequest(srv, http.MethodPost, "/api/v1/positions/"+first.ID+"/camera", `{"cameraId":"cam1"}`); rec.Code != http.StatusOK {
		t.Fatalf("assigning cam1 to first: expected 200, got %d", rec.Code)
	}
	if rec := doRequest(srv, http.MethodPost, "/api/v1/positions/"+second.ID+"/camera", `{"cameraId":"cam2"}`); rec.Code != http.StatusOK {
		t.Fatalf("assigning cam2 to second: expected 200, got %d", rec.Code)
	}

	if rec := doRequest(srv, http.MethodPost, "/api/v1/positions/"+first.ID+"/audio", ""); rec.Code != http.StatusOK {
		t.Fatalf("marking first as audio: expected 200, got %d", rec.Code)
	}
	if rec := doRequest(srv, http.MethodPost, "/api/v1/positions/"+second.ID+"/audio", ""); rec.Code != http.StatusOK {
		t.Fatalf("marking second as audio: expected 200, got %d", rec.Code)
	}

	firstInput := "pos_" + first.ID
	secondInput := "pos_" + second.ID
	if !obsCtl.Muted[firstInput] {
		t.Fatalf("expected first position input muted")
	}
	if obsCtl.Muted[secondInput] {
		t.Fatalf("expected second position input unmuted")
	}

	rec := doRequest(srv, http.MethodGet, "/api/v1/positions", "")
	var list []orchestrator.Position
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decoding list: %v", err)
	}
	byID := make(map[string]orchestrator.Position, len(list))
	for _, p := range list {
		byID[p.ID] = p
	}
	if byID[first.ID].CameraID != "cam1" || byID[second.ID].CameraID != "cam2" {
		t.Fatalf("expected visibility unaffected by audio change, got %+v", list)
	}
}

// IT-007: create a position via HTTP, then open a WS connection through the
// real ws.go handler; the initial burst must include a positions.updated
// envelope containing that position, alongside cameras.updated/system.status.
func TestIntegration_WSInitialSnapshot_IncludesPositions(t *testing.T) {
	srv, _, _, _, _ := newIntegrationServer(t)
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	pos := createPositionHTTP(t, srv, "Principal")

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/api/v1/ws?api_token=" + testToken
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dialing ws: %v", err)
	}
	defer conn.CloseNow()

	sawPositions := false
	var gotPositions []orchestrator.Position
	for i := 0; i < 3; i++ {
		var env wsEnvelope
		if err := wsjson.Read(ctx, conn, &env); err != nil {
			t.Fatalf("reading ws envelope %d: %v", i, err)
		}
		if env.Type == "positions.updated" {
			sawPositions = true
			raw, _ := json.Marshal(env.Payload)
			if err := json.Unmarshal(raw, &gotPositions); err != nil {
				t.Fatalf("decoding positions payload: %v", err)
			}
		}
	}

	if !sawPositions {
		t.Fatal("expected a positions.updated envelope in the initial snapshot")
	}
	if len(gotPositions) != 1 || gotPositions[0].ID != pos.ID {
		t.Fatalf("expected snapshot to contain created position, got %+v", gotPositions)
	}
}

// IT-008: configure obsmock.CreatePositionInput to fail; POST
// /api/v1/positions must 502, a subsequent GET must not include the failed
// position, and the underlying FileStore file must be byte-for-byte
// unchanged from before the call.
func TestIntegration_CreatePosition_OBSFailure_LeavesNoTrace(t *testing.T) {
	srv, orch, _, obsCtl, storePath := newIntegrationServer(t)

	// Establish a baseline persisted file with one existing position, so we
	// can assert it is untouched by the failed create.
	if _, err := orch.CreatePosition("Existente"); err != nil {
		t.Fatalf("seeding existing position: %v", err)
	}
	before, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("reading store file before failed create: %v", err)
	}

	obsCtl.CreatePositionInputErr = errors.New("obs unreachable")

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions", `{"name":"Falha"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(srv, http.MethodGet, "/api/v1/positions", "")
	var list []orchestrator.Position
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decoding list: %v", err)
	}
	for _, p := range list {
		if p.Name == "Falha" {
			t.Fatalf("expected failed position absent from list, got %+v", list)
		}
	}

	after, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("reading store file after failed create: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("expected store file unchanged; before=%q after=%q", before, after)
	}
}

// IT-009: write invalid JSON to the positions store file, construct the
// server's composition root against it; the server must start (not
// panic/exit) and GET /api/v1/positions must return 200 with an empty array.
func TestIntegration_CorruptPositionsFile_StartsEmpty(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "positions.json")
	if err := os.WriteFile(storePath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("writing corrupt store file: %v", err)
	}

	store := positions.NewFileStore(storePath)
	sstore := scenes.NewFileStore(filepath.Join(t.TempDir(), "scenes.json"))
	media := &fakeMediaClient{}
	obsCtl := obsmock.New()
	hub := events.NewHub()
	orch := orchestrator.New(media, obsCtl, hub, "Program", time.Second, "rtmp://localhost:1935", store, sstore)
	srv := NewServer(orch, hub, testToken, nil, nil, nil, nil, nil).Handler()

	rec := doRequest(srv, http.MethodGet, "/api/v1/positions", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Fatalf("expected empty array body, got %q", body)
	}
}

func decodeScene(t *testing.T, rec *httptest.ResponseRecorder) orchestrator.Scene {
	t.Helper()
	var sc orchestrator.Scene
	if err := json.NewDecoder(rec.Body).Decode(&sc); err != nil {
		t.Fatalf("decode scene: %v", err)
	}
	return sc
}

func decodeLive(t *testing.T, rec *httptest.ResponseRecorder) orchestrator.LiveState {
	t.Helper()
	var ls orchestrator.LiveState
	if err := json.NewDecoder(rec.Body).Decode(&ls); err != nil {
		t.Fatalf("decode live: %v", err)
	}
	return ls
}

func TestIntegration_Scenes_CRUD(t *testing.T) {
	srv, orch, media, obsCtl, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")
	pos := createPositionHTTP(t, srv, "Principal")
	_ = doRequest(srv, http.MethodPost, "/api/v1/positions/"+pos.ID+"/camera", `{"cameraId":"cam1"}`)

	// IT-009 empty list (before create)
	rec := doRequest(srv, http.MethodGet, "/api/v1/scenes", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list before create: %d", rec.Code)
	}

	// IT-001 create
	body := `{"name":"Abertura","positionIds":["` + pos.ID + `"]}`
	rec = doRequest(srv, http.MethodPost, "/api/v1/scenes", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	sc := decodeScene(t, rec)

	rec = doRequest(srv, http.MethodGet, "/api/v1/scenes", "")
	var list []orchestrator.Scene
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 || list[0].ID != sc.ID {
		t.Fatalf("list: %+v", list)
	}

	// IT-002 name taken
	rec = doRequest(srv, http.MethodPost, "/api/v1/scenes", `{"name":"Abertura","positionIds":[]}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}

	// IT-003 unknown position
	rec = doRequest(srv, http.MethodPost, "/api/v1/scenes", `{"name":"X","positionIds":["nope"]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	// IT-004 rename
	rec = doRequest(srv, http.MethodPatch, "/api/v1/scenes/"+sc.ID, `{"name":"Nova"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	if decodeScene(t, rec).Name != "Nova" {
		t.Fatalf("rename not applied")
	}

	// IT-005 rename collision
	sc2rec := doRequest(srv, http.MethodPost, "/api/v1/scenes", `{"name":"Outra","positionIds":[]}`)
	sc2 := decodeScene(t, sc2rec)
	rec = doRequest(srv, http.MethodPatch, "/api/v1/scenes/"+sc2.ID, `{"name":"Nova"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 collision, got %d", rec.Code)
	}

	// Preview+cut scene live for IT-006/007
	rec = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"scene","id":"`+sc.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("cut: %d %s", rec.Code, rec.Body.String())
	}

	// IT-006 live edit positions
	pos2 := createPositionHTTP(t, srv, "Secundaria")
	rec = doRequest(srv, http.MethodPatch, "/api/v1/scenes/"+sc.ID, `{"positionIds":["`+pos2.ID+`"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("live patch: %d %s", rec.Code, rec.Body.String())
	}
	if obsCtl.Enabled["pos_"+pos.ID] {
		t.Fatalf("old pos should be disabled after live edit")
	}
	if !obsCtl.Enabled["pos_"+pos2.ID] {
		t.Fatalf("new pos should be enabled after live edit")
	}

	// IT-007 delete live blocked
	rec = doRequest(srv, http.MethodDelete, "/api/v1/scenes/"+sc.ID, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}

	// cut away then IT-008 delete
	onlineCameras(media, orch, "cam1")
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	rec = doRequest(srv, http.MethodDelete, "/api/v1/scenes/"+sc.ID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
}

func TestIntegration_Scenes_EmptyList(t *testing.T) {
	srv, _, _, _, _ := newIntegrationServer(t)
	rec := doRequest(srv, http.MethodGet, "/api/v1/scenes", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("expected [], got %q", rec.Body.String())
	}
}

func TestIntegration_Live_PreviewCut(t *testing.T) {
	srv, orch, media, obsCtl, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")

	// IT-011 empty cut
	rec := doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 empty preview, got %d", rec.Code)
	}

	// IT-010 modo simples
	rec = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("cut: %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(srv, http.MethodGet, "/api/v1/live", "")
	ls := decodeLive(t, rec)
	if ls.LiveKind != orchestrator.LiveKindCamera || ls.LiveID != "cam1" {
		t.Fatalf("unexpected live: %+v", ls)
	}
	// no positions/scenes required
	rec = doRequest(srv, http.MethodGet, "/api/v1/positions", "")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("positions should stay empty")
	}
	rec = doRequest(srv, http.MethodGet, "/api/v1/scenes", "")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("scenes should stay empty")
	}
	if !obsCtl.Enabled["pos___simple__"] {
		t.Fatalf("hidden position should be enabled")
	}
}

func TestIntegration_Live_OfflineCutRejected(t *testing.T) {
	srv, orch, media, _, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
	media.set(nil)
	orch.SyncOnce(context.Background())
	rec := doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(srv, http.MethodGet, "/api/v1/live", "")
	ls := decodeLive(t, rec)
	if ls.PreviewID != "cam1" {
		t.Fatalf("preview retained: %+v", ls)
	}
}

func TestIntegration_Live_ModeAlternation(t *testing.T) {
	srv, orch, media, obsCtl, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")
	pos := createPositionHTTP(t, srv, "P1")
	_ = doRequest(srv, http.MethodPost, "/api/v1/positions/"+pos.ID+"/camera", `{"cameraId":"cam1"}`)
	rec := doRequest(srv, http.MethodPost, "/api/v1/scenes", `{"name":"S1","positionIds":["`+pos.ID+`"]}`)
	sc := decodeScene(t, rec)

	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	if !obsCtl.Enabled["pos___simple__"] {
		t.Fatalf("camera live: hidden enabled")
	}

	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"scene","id":"`+sc.ID+`"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	if obsCtl.Enabled["pos___simple__"] {
		t.Fatalf("scene live: hidden disabled")
	}
	if !obsCtl.Enabled["pos_"+pos.ID] {
		t.Fatalf("scene live: pos enabled")
	}

	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	rec = doRequest(srv, http.MethodGet, "/api/v1/live", "")
	ls := decodeLive(t, rec)
	if ls.LiveKind != orchestrator.LiveKindCamera {
		t.Fatalf("expected camera live again: %+v", ls)
	}
}

func TestIntegration_AssignCamera_NoImmediateEnable(t *testing.T) {
	srv, orch, media, obsCtl, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")
	pos := createPositionHTTP(t, srv, "P")
	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/"+pos.ID+"/camera", `{"cameraId":"cam1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("assign: %d", rec.Code)
	}
	if obsCtl.Enabled["pos_"+pos.ID] {
		t.Fatalf("must stay disabled until cut")
	}
}

func TestIntegration_WS_ScenesUpdated(t *testing.T) {
	srv, _, _, _, _ := newIntegrationServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Api-Token": []string{testToken}},
	})
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.CloseNow()

	// Drain snapshot
	for range 5 {
		var env wsEnvelope
		_ = wsjson.Read(ctx, conn, &env)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		doRequest(srv, http.MethodPost, "/api/v1/scenes", `{"name":"WS","positionIds":[]}`)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		readCtx, c := context.WithTimeout(ctx, 500*time.Millisecond)
		var env wsEnvelope
		err := wsjson.Read(readCtx, conn, &env)
		c()
		if err != nil {
			continue
		}
		if env.Type == "scenes.updated" {
			return
		}
	}
	t.Fatalf("timed out waiting for scenes.updated")
}

func TestIntegration_WS_LiveUpdated(t *testing.T) {
	srv, orch, media, _, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Api-Token": []string{testToken}},
	})
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.CloseNow()
	for range 5 {
		var env wsEnvelope
		_ = wsjson.Read(ctx, conn, &env)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
		doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		readCtx, c := context.WithTimeout(ctx, 500*time.Millisecond)
		var env wsEnvelope
		err := wsjson.Read(readCtx, conn, &env)
		c()
		if err != nil {
			continue
		}
		if env.Type == "live.updated" {
			return
		}
	}
	t.Fatalf("timed out waiting for live.updated")
}

func TestE2E_001_ModoSimplesZeroSetup(t *testing.T) {
	srv, orch, media, _, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	rec := doRequest(srv, http.MethodGet, "/api/v1/live", "")
	ls := decodeLive(t, rec)
	if ls.LiveKind != orchestrator.LiveKindCamera || ls.LiveID != "cam1" {
		t.Fatalf("live: %+v", ls)
	}
	if strings.TrimSpace(doRequest(srv, http.MethodGet, "/api/v1/positions", "").Body.String()) != "[]" {
		t.Fatalf("positions not empty")
	}
	if strings.TrimSpace(doRequest(srv, http.MethodGet, "/api/v1/scenes", "").Body.String()) != "[]" {
		t.Fatalf("scenes not empty")
	}
}

func TestE2E_002_ProducaoComCenas(t *testing.T) {
	srv, orch, media, obsCtl, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1", "cam2")
	p1 := createPositionHTTP(t, srv, "A")
	p2 := createPositionHTTP(t, srv, "B")
	_ = doRequest(srv, http.MethodPost, "/api/v1/positions/"+p1.ID+"/camera", `{"cameraId":"cam1"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/positions/"+p2.ID+"/camera", `{"cameraId":"cam2"}`)
	rec := doRequest(srv, http.MethodPost, "/api/v1/scenes", `{"name":"Show","positionIds":["`+p1.ID+`","`+p2.ID+`"]}`)
	sc := decodeScene(t, rec)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"scene","id":"`+sc.ID+`"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	if !obsCtl.Enabled["pos_"+p1.ID] || !obsCtl.Enabled["pos_"+p2.ID] {
		t.Fatalf("both should be enabled")
	}
	_ = doRequest(srv, http.MethodPatch, "/api/v1/scenes/"+sc.ID, `{"positionIds":["`+p1.ID+`"]}`)
	if obsCtl.Enabled["pos_"+p2.ID] {
		t.Fatalf("p2 should disable immediately")
	}
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	ls := decodeLive(t, doRequest(srv, http.MethodGet, "/api/v1/live", ""))
	if ls.LiveKind != orchestrator.LiveKindCamera {
		t.Fatalf("expected camera: %+v", ls)
	}
	if obsCtl.Enabled["pos_"+p1.ID] {
		t.Fatalf("scene positions should be off")
	}
}

func TestE2E_004_DeleteBlockedThenAllowed(t *testing.T) {
	srv, orch, media, _, _ := newIntegrationServer(t)
	onlineCameras(media, orch, "cam1")
	rec := doRequest(srv, http.MethodPost, "/api/v1/scenes", `{"name":"LiveMe","positionIds":[]}`)
	sc := decodeScene(t, rec)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"scene","id":"`+sc.ID+`"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	rec = doRequest(srv, http.MethodDelete, "/api/v1/scenes/"+sc.ID, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
	_ = doRequest(srv, http.MethodPost, "/api/v1/live/cut", "")
	rec = doRequest(srv, http.MethodDelete, "/api/v1/scenes/"+sc.ID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d %s", rec.Code, rec.Body.String())
	}
}
