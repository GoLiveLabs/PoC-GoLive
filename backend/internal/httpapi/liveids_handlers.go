package httpapi

import (
	"net/http"

	"live-orchestrator/backend/internal/liveid"
)

func (s *Server) handleCreateLiveID(w http.ResponseWriter, r *http.Request) {
	clientID, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	var req liveid.CreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	l, err := s.liveIDs.Create(r.Context(), clientID, req)
	if err != nil {
		writeLiveIDError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, liveid.ToResponse(l))
}

func (s *Server) handleListLiveIDsByClient(w http.ResponseWriter, r *http.Request) {
	clientID, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	s.listLiveIDs(w, r, liveid.ListFilter{ClientID: &clientID})
}

func (s *Server) handleListLiveIDsFlat(w http.ResponseWriter, r *http.Request) {
	clientID, ok := queryUUID(w, r, "clientId")
	if !ok {
		return
	}
	s.listLiveIDs(w, r, liveid.ListFilter{ClientID: clientID})
}

func (s *Server) listLiveIDs(w http.ResponseWriter, r *http.Request, filter liveid.ListFilter) {
	platformID, ok := queryUUID(w, r, "platformId")
	if !ok {
		return
	}
	filter.PlatformID = platformID

	isActive, ok := queryBool(w, r, "isActive")
	if !ok {
		return
	}
	filter.IsActive = isActive

	page, ok := parsePage(w, r)
	if !ok {
		return
	}

	result, err := s.liveIDs.List(r.Context(), filter, page)
	if err != nil {
		writeLiveIDError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetLiveID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	l, err := s.liveIDs.GetByID(r.Context(), id)
	if err != nil {
		writeLiveIDError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, liveid.ToResponse(l))
}

func (s *Server) handleUpdateLiveID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req liveid.UpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	l, err := s.liveIDs.Update(r.Context(), id, req)
	if err != nil {
		writeLiveIDError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, liveid.ToResponse(l))
}

func (s *Server) handleDeleteLiveID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := s.liveIDs.Delete(r.Context(), id); err != nil {
		writeLiveIDError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeLiveIDError(w http.ResponseWriter, err error) {
	switch {
	case isErr(err, liveid.ErrNotFound):
		writeError(w, http.StatusNotFound, "live id not found")
	case isErr(err, liveid.ErrClientNotFound):
		writeError(w, http.StatusNotFound, "client not found")
	case isErr(err, liveid.ErrPlatformNotFound):
		writeError(w, http.StatusNotFound, "streaming platform not found")
	case isErr(err, liveid.ErrDuplicateLiveID):
		writeError(w, http.StatusConflict, "this live id already exists for the client and platform")
	case isErr(err, liveid.ErrInvalidLiveID):
		writeValidationError(w, "liveId", err.Error())
	default:
		writeInternalError(w, err)
	}
}
