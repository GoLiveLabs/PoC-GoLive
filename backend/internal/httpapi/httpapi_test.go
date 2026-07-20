package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/orchestrator"
)

const testToken = "test-token"

// fakeOrchestrator implements Orchestrator for handler tests.
type fakeOrchestrator struct {
	cameras    []orchestrator.Camera
	status     orchestrator.SystemStatus
	setLiveErr error
}

func (f *fakeOrchestrator) Cameras() []orchestrator.Camera { return f.cameras }
func (f *fakeOrchestrator) Status() orchestrator.SystemStatus { return f.status }
func (f *fakeOrchestrator) SyncOnce(ctx context.Context) []orchestrator.Camera { return f.cameras }
func (f *fakeOrchestrator) SetLive(cameraID string) (orchestrator.SystemStatus, error) {
	if f.setLiveErr != nil {
		return orchestrator.SystemStatus{}, f.setLiveErr
	}
	f.status.LiveCameraID = cameraID
	return f.status, nil
}

func newTestServer(orch *fakeOrchestrator) http.Handler {
	hub := events.NewHub()
	return NewServer(orch, hub, testToken, nil, nil, nil, nil).Handler()
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

func TestSetLive_UnknownCamera_404(t *testing.T) {
	orch := &fakeOrchestrator{setLiveErr: orchestrator.ErrCameraNotFound}
	srv := newTestServer(orch)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cameras/ghost/live", nil)
	req.Header.Set("X-Api-Token", testToken)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSetLive_OfflineCamera_409(t *testing.T) {
	orch := &fakeOrchestrator{setLiveErr: orchestrator.ErrCameraOffline}
	srv := newTestServer(orch)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cameras/camera1/live", nil)
	req.Header.Set("X-Api-Token", testToken)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestSetLive_ObsUnreachable_502(t *testing.T) {
	orch := &fakeOrchestrator{setLiveErr: orchestrator.ErrOBSUnreachable}
	srv := newTestServer(orch)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cameras/camera1/live", nil)
	req.Header.Set("X-Api-Token", testToken)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestSetLive_HappyPath(t *testing.T) {
	orch := &fakeOrchestrator{
		cameras: []orchestrator.Camera{{ID: "camera1", Status: "online"}},
		status:  orchestrator.SystemStatus{ActiveSceneName: "Program"},
	}
	srv := newTestServer(orch)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cameras/camera1/live", nil)
	req.Header.Set("X-Api-Token", testToken)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var status orchestrator.SystemStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if status.LiveCameraID != "camera1" {
		t.Fatalf("expected camera1 live, got %q", status.LiveCameraID)
	}
}
