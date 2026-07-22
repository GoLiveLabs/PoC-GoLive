package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/orchestrator"
)

const testToken = "test-token"

// fakeOrchestrator implements Orchestrator for handler tests.
type fakeOrchestrator struct {
	cameras []orchestrator.Camera
	status  orchestrator.SystemStatus

	positions []orchestrator.Position
	// resultPosition, when position-mutating methods succeed, is returned as
	// the resulting Position (defaults to the zero value if unset).
	resultPosition orchestrator.Position

	scenes       []orchestrator.Scene
	resultScene  orchestrator.Scene
	liveState    orchestrator.LiveState
	resultLive   orchestrator.LiveState

	// err, when set, is returned by every mutating method call
	// instead of the normal success path, letting tests inject any failure
	// shape from the error-to-status mapping table.
	err error
}

func (f *fakeOrchestrator) Cameras() []orchestrator.Camera                     { return f.cameras }
func (f *fakeOrchestrator) Status() orchestrator.SystemStatus                  { return f.status }
func (f *fakeOrchestrator) SyncOnce(ctx context.Context) []orchestrator.Camera { return f.cameras }

func (f *fakeOrchestrator) Positions() []orchestrator.Position { return f.positions }

func (f *fakeOrchestrator) CreatePosition(name string) (orchestrator.Position, error) {
	if f.err != nil {
		return orchestrator.Position{}, f.err
	}
	return f.resultPosition, nil
}

func (f *fakeOrchestrator) RenamePosition(id, newName string) (orchestrator.Position, error) {
	if f.err != nil {
		return orchestrator.Position{}, f.err
	}
	return f.resultPosition, nil
}

func (f *fakeOrchestrator) DeletePosition(id string) error {
	return f.err
}

func (f *fakeOrchestrator) AssignCamera(positionID, cameraID string) (orchestrator.Position, error) {
	if f.err != nil {
		return orchestrator.Position{}, f.err
	}
	return f.resultPosition, nil
}

func (f *fakeOrchestrator) UnassignPosition(positionID string) (orchestrator.Position, error) {
	if f.err != nil {
		return orchestrator.Position{}, f.err
	}
	return f.resultPosition, nil
}

func (f *fakeOrchestrator) SetAudioPosition(positionID string) (orchestrator.Position, error) {
	if f.err != nil {
		return orchestrator.Position{}, f.err
	}
	return f.resultPosition, nil
}

func (f *fakeOrchestrator) Scenes() []orchestrator.Scene { return f.scenes }

func (f *fakeOrchestrator) CreateScene(name string, positionIDs []string) (orchestrator.Scene, error) {
	if f.err != nil {
		return orchestrator.Scene{}, f.err
	}
	return f.resultScene, nil
}

func (f *fakeOrchestrator) RenameScene(id, newName string) (orchestrator.Scene, error) {
	if f.err != nil {
		return orchestrator.Scene{}, f.err
	}
	return f.resultScene, nil
}

func (f *fakeOrchestrator) UpdateScenePositions(id string, positionIDs []string) (orchestrator.Scene, error) {
	if f.err != nil {
		return orchestrator.Scene{}, f.err
	}
	return f.resultScene, nil
}

func (f *fakeOrchestrator) DeleteScene(id string) error {
	return f.err
}

func (f *fakeOrchestrator) LiveState() orchestrator.LiveState { return f.liveState }

func (f *fakeOrchestrator) SetPreviewCamera(cameraID string) (orchestrator.LiveState, error) {
	if f.err != nil {
		return orchestrator.LiveState{}, f.err
	}
	return f.resultLive, nil
}

func (f *fakeOrchestrator) SetPreviewScene(sceneID string) (orchestrator.LiveState, error) {
	if f.err != nil {
		return orchestrator.LiveState{}, f.err
	}
	return f.resultLive, nil
}

func (f *fakeOrchestrator) Cut(ctx context.Context) (orchestrator.LiveState, error) {
	if f.err != nil {
		return orchestrator.LiveState{}, f.err
	}
	return f.resultLive, nil
}

func newTestServer(orch *fakeOrchestrator) http.Handler {
	hub := events.NewHub()
	return NewServer(orch, hub, testToken, nil, nil, nil, nil, nil).Handler()
}

func TestHealth_NoTokenRequired(t *testing.T) {
	srv := newTestServer(&fakeOrchestrator{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestCameras_NoToken_Unauthorized(t *testing.T) {
	srv := newTestServer(&fakeOrchestrator{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cameras", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCameras_WithToken_OK(t *testing.T) {
	orch := &fakeOrchestrator{cameras: []orchestrator.Camera{{ID: "camera1", Status: "online"}}}
	srv := newTestServer(orch)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cameras", nil)
	req.Header.Set("X-Api-Token", testToken)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var cams []orchestrator.Camera
	if err := json.NewDecoder(rec.Body).Decode(&cams); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(cams) != 1 || cams[0].ID != "camera1" {
		t.Fatalf("unexpected cameras: %+v", cams)
	}
}

func TestSetLiveRoute_Removed_404(t *testing.T) {
	srv := newTestServer(&fakeOrchestrator{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cameras/camera1/live", nil)
	req.Header.Set("X-Api-Token", testToken)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (route removed), got %d", rec.Code)
	}
}

// doRequest issues a request against srv with the test token set and,
// when body is non-empty, a JSON content type.
func doRequest(srv http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("X-Api-Token", testToken)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func decodePosition(t *testing.T, rec *httptest.ResponseRecorder) orchestrator.Position {
	t.Helper()
	var pos orchestrator.Position
	if err := json.NewDecoder(rec.Body).Decode(&pos); err != nil {
		t.Fatalf("decoding position response: %v", err)
	}
	return pos
}

// UT-051: POST /api/v1/positions with a valid name -> 201 with the created Position.
func TestCreatePosition_Happy_201(t *testing.T) {
	orch := &fakeOrchestrator{resultPosition: orchestrator.Position{ID: "p1", Name: "Principal"}}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions", `{"name":"Principal"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	pos := decodePosition(t, rec)
	if pos.ID != "p1" || pos.Name != "Principal" {
		t.Fatalf("unexpected position: %+v", pos)
	}
}

// UT-052: POST /api/v1/positions with an empty name -> 422.
func TestCreatePosition_EmptyName_422(t *testing.T) {
	orch := &fakeOrchestrator{err: orchestrator.ErrPositionNameEmpty}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions", `{"name":""}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
}

// UT-053: POST /api/v1/positions with a duplicate name -> 409.
func TestCreatePosition_NameTaken_409(t *testing.T) {
	orch := &fakeOrchestrator{err: orchestrator.ErrPositionNameTaken}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions", `{"name":"Principal"}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

// UT-054: GET /api/v1/positions with two positions created -> 200 with a 2-element array.
func TestListPositions_Happy_TwoEntries(t *testing.T) {
	orch := &fakeOrchestrator{positions: []orchestrator.Position{{ID: "p1"}, {ID: "p2"}}}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodGet, "/api/v1/positions", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []orchestrator.Position
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(got))
	}
}

// UT-055: GET /api/v1/positions with none created -> 200 with an empty array, never null.
func TestListPositions_Empty_NeverNull(t *testing.T) {
	orch := &fakeOrchestrator{positions: []orchestrator.Position{}}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodGet, "/api/v1/positions", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Fatalf("expected body `[]`, got %q", body)
	}
}

// UT-056: PATCH /api/v1/positions/{id} with a new name -> 200 with the updated Position.
func TestRenamePosition_Happy_200(t *testing.T) {
	orch := &fakeOrchestrator{resultPosition: orchestrator.Position{ID: "p1", Name: "Novo Nome"}}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPatch, "/api/v1/positions/p1", `{"name":"Novo Nome"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	pos := decodePosition(t, rec)
	if pos.Name != "Novo Nome" {
		t.Fatalf("unexpected position: %+v", pos)
	}
}

// UT-057: PATCH /api/v1/positions/{unknown-id} -> 404.
func TestRenamePosition_UnknownID_404(t *testing.T) {
	orch := &fakeOrchestrator{err: orchestrator.ErrPositionNotFound}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPatch, "/api/v1/positions/unknown", `{"name":"Novo Nome"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// UT-058: DELETE /api/v1/positions/{id} -> 204.
func TestDeletePosition_Happy_204(t *testing.T) {
	orch := &fakeOrchestrator{}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodDelete, "/api/v1/positions/p1", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

// UT-059: DELETE /api/v1/positions/{unknown-id} -> 404.
func TestDeletePosition_UnknownID_404(t *testing.T) {
	orch := &fakeOrchestrator{err: orchestrator.ErrPositionNotFound}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodDelete, "/api/v1/positions/unknown", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// UT-060: POST /api/v1/positions/{id}/camera with a valid cameraId -> 200 with cameraId assigned.
func TestAssignCamera_Happy_200(t *testing.T) {
	orch := &fakeOrchestrator{resultPosition: orchestrator.Position{ID: "p1", CameraID: "cam1"}}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/p1/camera", `{"cameraId":"cam1"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	pos := decodePosition(t, rec)
	if pos.CameraID != "cam1" {
		t.Fatalf("expected cameraId cam1, got %+v", pos)
	}
}

// UT-061: POST /api/v1/positions/{id}/camera with an unknown cameraId -> 404.
func TestAssignCamera_UnknownCamera_404(t *testing.T) {
	orch := &fakeOrchestrator{err: orchestrator.ErrCameraNotFound}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/p1/camera", `{"cameraId":"unknown"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// UT-062: POST /api/v1/positions/{id}/camera for an offline camera -> 409.
func TestAssignCamera_OfflineCamera_409(t *testing.T) {
	orch := &fakeOrchestrator{err: orchestrator.ErrCameraOffline}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/p1/camera", `{"cameraId":"cam1"}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

// UT-063: POST /api/v1/positions/{id}/camera when the orchestrator returns
// ErrPositionOBSInputMissing -> 502.
func TestAssignCamera_OBSInputMissing_502(t *testing.T) {
	orch := &fakeOrchestrator{err: orchestrator.ErrPositionOBSInputMissing}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/p1/camera", `{"cameraId":"cam1"}`)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

// UT-064: DELETE /api/v1/positions/{id}/camera -> 200 with Position.cameraId == "".
func TestUnassignPosition_Happy_200(t *testing.T) {
	orch := &fakeOrchestrator{resultPosition: orchestrator.Position{ID: "p1", CameraID: ""}}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodDelete, "/api/v1/positions/p1/camera", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	pos := decodePosition(t, rec)
	if pos.CameraID != "" {
		t.Fatalf("expected empty cameraId, got %+v", pos)
	}
}

// UT-065: POST /api/v1/positions/{id}/audio -> 200 with Position.isAudioSource == true.
func TestSetAudioPosition_Happy_200(t *testing.T) {
	orch := &fakeOrchestrator{resultPosition: orchestrator.Position{ID: "p1", IsAudioSource: true}}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/p1/audio", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	pos := decodePosition(t, rec)
	if !pos.IsAudioSource {
		t.Fatalf("expected isAudioSource true, got %+v", pos)
	}
}

// UT-066: POST /api/v1/positions/{id}/audio on a position with cameraId == "" -> 422.
func TestSetAudioPosition_PositionEmpty_422(t *testing.T) {
	orch := &fakeOrchestrator{err: orchestrator.ErrPositionEmpty}
	srv := newTestServer(orch)

	rec := doRequest(srv, http.MethodPost, "/api/v1/positions/p1/audio", "")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
}

// UT-067: POST /api/v1/cameras/{id}/live (removed route) -> 404 (route not registered).
// Covered by TestSetLiveRoute_Removed_404 above.
