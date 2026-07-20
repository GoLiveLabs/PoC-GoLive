// Package httpapi exposes the orchestrator over a REST + WebSocket HTTP API.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/coder/websocket"

	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/orchestrator"
)

// Orchestrator is the subset of orchestrator.Orchestrator the HTTP layer depends on.
type Orchestrator interface {
	Cameras() []orchestrator.Camera
	Status() orchestrator.SystemStatus
	SyncOnce(ctx context.Context) []orchestrator.Camera
	SetLive(cameraID string) (orchestrator.SystemStatus, error)
}

// Server wires the orchestrator and the client/ingest/streaming-platform/
// live-id domain services to HTTP handlers.
type Server struct {
	orch  Orchestrator
	hub   *events.Hub
	token string
	mux   *http.ServeMux

	clients   ClientService
	ingests   IngestService
	platforms StreamPlatformService
	liveIDs   LiveIDService

	connsMu sync.Mutex
	conns   map[*websocket.Conn]struct{}
}

// NewServer creates a Server. Call Handler to get the http.Handler to serve.
func NewServer(
	orch Orchestrator,
	hub *events.Hub,
	apiToken string,
	clients ClientService,
	ingests IngestService,
	platforms StreamPlatformService,
	liveIDs LiveIDService,
) *Server {
	s := &Server{
		orch:      orch,
		hub:       hub,
		token:     apiToken,
		clients:   clients,
		ingests:   ingests,
		platforms: platforms,
		liveIDs:   liveIDs,
		conns:     make(map[*websocket.Conn]struct{}),
	}
	s.routes()
	return s
}

// trackConn registers an open WebSocket connection so CloseAllWS can close it
// during graceful shutdown. The returned func must be called (typically via
// defer) when the connection's handler returns.
func (s *Server) trackConn(c *websocket.Conn) func() {
	s.connsMu.Lock()
	s.conns[c] = struct{}{}
	s.connsMu.Unlock()
	return func() {
		s.connsMu.Lock()
		delete(s.conns, c)
		s.connsMu.Unlock()
	}
}

// CloseAllWS closes every currently open WebSocket connection. Call this
// before or during http.Server.Shutdown so long-lived WS handlers return
// and the shutdown doesn't hang.
func (s *Server) CloseAllWS() {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	for c := range s.conns {
		_ = c.Close(websocket.StatusServiceRestart, "server shutting down")
	}
}

// Handler returns the fully wrapped http.Handler (routes + middleware).
func (s *Server) Handler() http.Handler {
	return corsMiddleware(s.authMiddleware(s.mux))
}

func (s *Server) routes() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/cameras", s.handleListCameras)
	mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	mux.HandleFunc("POST /api/v1/cameras/{id}/live", s.handleSetLive)
	mux.HandleFunc("POST /api/v1/sync", s.handleSync)
	mux.HandleFunc("GET /api/v1/ws", s.handleWS)

	mux.HandleFunc("POST /api/v1/clients", s.handleCreateClient)
	mux.HandleFunc("GET /api/v1/clients", s.handleListClients)
	mux.HandleFunc("GET /api/v1/clients/{id}", s.handleGetClient)
	mux.HandleFunc("PATCH /api/v1/clients/{id}", s.handleUpdateClient)
	mux.HandleFunc("DELETE /api/v1/clients/{id}", s.handleDeleteClient)

	mux.HandleFunc("POST /api/v1/clients/{clientID}/ingests", s.handleCreateIngest)
	mux.HandleFunc("GET /api/v1/clients/{clientID}/ingests", s.handleListIngestsByClient)
	mux.HandleFunc("GET /api/v1/ingests", s.handleListIngestsFlat)
	mux.HandleFunc("GET /api/v1/ingests/{id}", s.handleGetIngest)
	mux.HandleFunc("PATCH /api/v1/ingests/{id}", s.handleUpdateIngest)
	mux.HandleFunc("DELETE /api/v1/ingests/{id}", s.handleDeleteIngest)

	mux.HandleFunc("POST /api/v1/streaming-platforms", s.handleCreatePlatform)
	mux.HandleFunc("GET /api/v1/streaming-platforms", s.handleListPlatforms)
	mux.HandleFunc("GET /api/v1/streaming-platforms/{id}", s.handleGetPlatform)
	mux.HandleFunc("PATCH /api/v1/streaming-platforms/{id}", s.handleUpdatePlatform)
	mux.HandleFunc("DELETE /api/v1/streaming-platforms/{id}", s.handleDeletePlatform)

	mux.HandleFunc("POST /api/v1/clients/{clientID}/live-ids", s.handleCreateLiveID)
	mux.HandleFunc("GET /api/v1/clients/{clientID}/live-ids", s.handleListLiveIDsByClient)
	mux.HandleFunc("GET /api/v1/live-ids", s.handleListLiveIDsFlat)
	mux.HandleFunc("GET /api/v1/live-ids/{id}", s.handleGetLiveID)
	mux.HandleFunc("PATCH /api/v1/live-ids/{id}", s.handleUpdateLiveID)
	mux.HandleFunc("DELETE /api/v1/live-ids/{id}", s.handleDeleteLiveID)

	s.mux = mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListCameras(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.orch.Cameras())
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.orch.Status())
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	cams := s.orch.SyncOnce(r.Context())
	writeJSON(w, http.StatusOK, cams)
}

func (s *Server) handleSetLive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status, err := s.orch.SetLive(id)
	if err == nil {
		writeJSON(w, http.StatusOK, status)
		return
	}

	switch {
	case isErr(err, orchestrator.ErrCameraNotFound):
		writeError(w, http.StatusNotFound, "câmera não encontrada")
	case isErr(err, orchestrator.ErrCameraOffline):
		writeError(w, http.StatusConflict, "câmera está offline")
	case isErr(err, orchestrator.ErrOBSUnreachable):
		writeError(w, http.StatusBadGateway, "OBS está inacessível no momento")
	default:
		writeError(w, http.StatusInternalServerError, "erro interno")
	}
}
