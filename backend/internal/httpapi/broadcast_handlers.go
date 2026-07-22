package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/broadcast"
	"live-orchestrator/backend/internal/client"
)

type setBroadcastClientRequest struct {
	ClientID uuid.UUID `json:"clientId"`
}

func (s *Server) handleGetBroadcast(w http.ResponseWriter, r *http.Request) {
	if s.broadcast == nil {
		writeJSON(w, http.StatusOK, broadcast.StatusSnapshot{Destinations: []broadcast.DestinationStatus{}})
		return
	}
	writeJSON(w, http.StatusOK, s.broadcast.Snapshot())
}

func (s *Server) handleSetBroadcastClient(w http.ResponseWriter, r *http.Request) {
	if s.broadcast == nil {
		writeError(w, http.StatusInternalServerError, "broadcast not configured")
		return
	}
	var req setBroadcastClientRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.clients != nil {
		if _, err := s.clients.GetByID(r.Context(), req.ClientID); err != nil {
			writeBroadcastError(w, err)
			return
		}
	}
	if err := s.broadcast.SetActiveClient(r.Context(), req.ClientID); err != nil {
		writeBroadcastError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.broadcast.Snapshot())
}

func (s *Server) handleBroadcastStart(w http.ResponseWriter, r *http.Request) {
	if s.broadcast == nil {
		writeError(w, http.StatusInternalServerError, "broadcast not configured")
		return
	}
	if err := s.broadcast.Start(r.Context()); err != nil {
		writeBroadcastError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.broadcast.Snapshot())
}

func (s *Server) handleBroadcastStop(w http.ResponseWriter, r *http.Request) {
	if s.broadcast == nil {
		writeJSON(w, http.StatusOK, broadcast.StatusSnapshot{Destinations: []broadcast.DestinationStatus{}})
		return
	}
	if err := s.broadcast.Stop(r.Context()); err != nil {
		writeBroadcastError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.broadcast.Snapshot())
}

func (s *Server) handleBroadcastRestart(w http.ResponseWriter, r *http.Request) {
	if s.broadcast == nil {
		writeError(w, http.StatusInternalServerError, "broadcast not configured")
		return
	}
	liveID, ok := pathUUID(w, r, "liveId")
	if !ok {
		return
	}
	if err := s.broadcast.RestartDestination(r.Context(), liveID); err != nil {
		writeBroadcastError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.broadcast.Snapshot())
}

func writeBroadcastError(w http.ResponseWriter, err error) {
	switch {
	case isErr(err, client.ErrNotFound):
		writeError(w, http.StatusNotFound, "client not found")
	case isErr(err, broadcast.ErrClientSwitchWhileBroadcasting):
		writeError(w, http.StatusConflict, "cannot switch client while broadcasting")
	case isErr(err, broadcast.ErrNoActiveClient):
		writeError(w, http.StatusConflict, "no active client")
	case isErr(err, broadcast.ErrNoDestinationsConfigured):
		writeError(w, http.StatusConflict, "no destinations configured")
	case isErr(err, broadcast.ErrNothingLive):
		writeError(w, http.StatusConflict, "nothing live")
	case isErr(err, broadcast.ErrDestinationNotFound):
		writeError(w, http.StatusNotFound, "destination not found")
	case isErr(err, broadcast.ErrBroadcastStopped):
		writeError(w, http.StatusConflict, "broadcast is stopped")
	default:
		writeInternalError(w, err)
	}
}
