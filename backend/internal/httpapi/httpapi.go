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

	Positions() []orchestrator.Position
	CreatePosition(name string) (orchestrator.Position, error)
	RenamePosition(id, newName string) (orchestrator.Position, error)
	DeletePosition(id string) error
	AssignCamera(positionID, cameraID string) (orchestrator.Position, error)
	UnassignPosition(positionID string) (orchestrator.Position, error)
	SetAudioPosition(positionID string) (orchestrator.Position, error)

	Scenes() []orchestrator.Scene
	CreateScene(name string, positionIDs []string) (orchestrator.Scene, error)
	RenameScene(id, newName string) (orchestrator.Scene, error)
	UpdateScenePositions(id string, positionIDs []string) (orchestrator.Scene, error)
	DeleteScene(id string) error

	LiveState() orchestrator.LiveState
	SetPreviewCamera(cameraID string) (orchestrator.LiveState, error)
	SetPreviewScene(sceneID string) (orchestrator.LiveState, error)
	Cut(ctx context.Context) (orchestrator.LiveState, error)
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
	broadcast BroadcastService

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
	broadcastSvc BroadcastService,
) *Server {
	s := &Server{
		orch:      orch,
		hub:       hub,
		token:     apiToken,
		clients:   clients,
		ingests:   ingests,
		platforms: platforms,
		liveIDs:   liveIDs,
		broadcast: broadcastSvc,
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

	mux.HandleFunc("GET /api/v1/positions", s.handleListPositions)
	mux.HandleFunc("POST /api/v1/positions", s.handleCreatePosition)
	mux.HandleFunc("PATCH /api/v1/positions/{id}", s.handleRenamePosition)
	mux.HandleFunc("DELETE /api/v1/positions/{id}", s.handleDeletePosition)
	mux.HandleFunc("POST /api/v1/positions/{id}/camera", s.handleAssignCamera)
	mux.HandleFunc("DELETE /api/v1/positions/{id}/camera", s.handleUnassignPosition)
	mux.HandleFunc("POST /api/v1/positions/{id}/audio", s.handleSetAudioPosition)

	mux.HandleFunc("GET /api/v1/scenes", s.handleListScenes)
	mux.HandleFunc("POST /api/v1/scenes", s.handleCreateScene)
	mux.HandleFunc("PATCH /api/v1/scenes/{id}", s.handlePatchScene)
	mux.HandleFunc("DELETE /api/v1/scenes/{id}", s.handleDeleteScene)

	mux.HandleFunc("GET /api/v1/live", s.handleGetLive)
	mux.HandleFunc("POST /api/v1/live/preview", s.handleSetPreview)
	mux.HandleFunc("POST /api/v1/live/cut", s.handleCut)

	mux.HandleFunc("GET /api/v1/broadcast", s.handleGetBroadcast)
	mux.HandleFunc("POST /api/v1/broadcast/client", s.handleSetBroadcastClient)
	mux.HandleFunc("POST /api/v1/broadcast/start", s.handleBroadcastStart)
	mux.HandleFunc("POST /api/v1/broadcast/stop", s.handleBroadcastStop)
	mux.HandleFunc("POST /api/v1/broadcast/destinations/{liveId}/restart", s.handleBroadcastRestart)

	s.mux = mux
}

// createPositionRequest is the body of POST /api/v1/positions.
type createPositionRequest struct {
	Name string `json:"name"`
}

// renamePositionRequest is the body of PATCH /api/v1/positions/{id}.
type renamePositionRequest struct {
	Name string `json:"name"`
}

// assignCameraRequest is the body of POST /api/v1/positions/{id}/camera.
type assignCameraRequest struct {
	CameraID string `json:"cameraId"`
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

func (s *Server) handleListPositions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.orch.Positions())
}

func (s *Server) handleCreatePosition(w http.ResponseWriter, r *http.Request) {
	var req createPositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	pos, err := s.orch.CreatePosition(req.Name)
	if err == nil {
		writeJSON(w, http.StatusCreated, pos)
		return
	}
	writePositionError(w, err)
}

func (s *Server) handleRenamePosition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req renamePositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	pos, err := s.orch.RenamePosition(id, req.Name)
	if err == nil {
		writeJSON(w, http.StatusOK, pos)
		return
	}
	writePositionError(w, err)
}

func (s *Server) handleDeletePosition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.orch.DeletePosition(id)
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writePositionError(w, err)
}

func (s *Server) handleAssignCamera(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req assignCameraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	pos, err := s.orch.AssignCamera(id, req.CameraID)
	if err == nil {
		writeJSON(w, http.StatusOK, pos)
		return
	}
	writePositionError(w, err)
}

func (s *Server) handleUnassignPosition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pos, err := s.orch.UnassignPosition(id)
	if err == nil {
		writeJSON(w, http.StatusOK, pos)
		return
	}
	writePositionError(w, err)
}

func (s *Server) handleSetAudioPosition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pos, err := s.orch.SetAudioPosition(id)
	if err == nil {
		writeJSON(w, http.StatusOK, pos)
		return
	}
	writePositionError(w, err)
}

// writePositionError maps a position/camera sentinel error to its HTTP
// status code and Portuguese message, per _techspec.md's API Endpoints
// error-to-status table.
func writePositionError(w http.ResponseWriter, err error) {
	switch {
	case isErr(err, orchestrator.ErrPositionNotFound):
		writeError(w, http.StatusNotFound, "posição não encontrada")
	case isErr(err, orchestrator.ErrCameraNotFound):
		writeError(w, http.StatusNotFound, "câmera não encontrada")
	case isErr(err, orchestrator.ErrCameraOffline):
		writeError(w, http.StatusConflict, "câmera está offline")
	case isErr(err, orchestrator.ErrPositionNameTaken):
		writeError(w, http.StatusConflict, "nome de posição já utilizado")
	case isErr(err, orchestrator.ErrPositionNameEmpty):
		writeError(w, http.StatusUnprocessableEntity, "nome de posição não pode ser vazio")
	case isErr(err, orchestrator.ErrPositionNameTooLong):
		writeError(w, http.StatusUnprocessableEntity, "nome de posição excede o tamanho máximo permitido")
	case isErr(err, orchestrator.ErrPositionEmpty):
		writeError(w, http.StatusUnprocessableEntity, "posição não possui câmera atribuída")
	case isErr(err, orchestrator.ErrOBSUnreachable):
		writeError(w, http.StatusBadGateway, "OBS está inacessível no momento")
	case isErr(err, orchestrator.ErrPositionOBSInputMissing):
		writeError(w, http.StatusBadGateway, "input do OBS da posição não encontrado")
	case isErr(err, orchestrator.ErrReservedPosition):
		writeError(w, http.StatusUnprocessableEntity, "posição reservada")
	default:
		writeError(w, http.StatusInternalServerError, "erro interno")
	}
}
