package httpapi

import (
	"net/http"

	"live-orchestrator/backend/internal/streamplatform"
)

func (s *Server) handleCreatePlatform(w http.ResponseWriter, r *http.Request) {
	var req streamplatform.CreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	p, err := s.platforms.Create(r.Context(), req)
	if err != nil {
		writePlatformError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, streamplatform.ToResponse(p))
}

func (s *Server) handleListPlatforms(w http.ResponseWriter, r *http.Request) {
	page, ok := parsePage(w, r)
	if !ok {
		return
	}
	result, err := s.platforms.List(r.Context(), page)
	if err != nil {
		writePlatformError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetPlatform(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, err := s.platforms.GetByID(r.Context(), id)
	if err != nil {
		writePlatformError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, streamplatform.ToResponse(p))
}

func (s *Server) handleUpdatePlatform(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req streamplatform.UpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	p, err := s.platforms.Update(r.Context(), id, req)
	if err != nil {
		writePlatformError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, streamplatform.ToResponse(p))
}

func (s *Server) handleDeletePlatform(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := s.platforms.Delete(r.Context(), id); err != nil {
		writePlatformError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writePlatformError(w http.ResponseWriter, err error) {
	switch {
	case isErr(err, streamplatform.ErrNotFound):
		writeError(w, http.StatusNotFound, "streaming platform not found")
	case isErr(err, streamplatform.ErrDuplicateSlug):
		writeError(w, http.StatusConflict, "a streaming platform with this slug already exists")
	case isErr(err, streamplatform.ErrPlatformInUse):
		writeError(w, http.StatusConflict, "streaming platform is referenced by existing live ids")
	case isErr(err, streamplatform.ErrInvalidSlug):
		writeValidationError(w, "slug", err.Error())
	case isErr(err, streamplatform.ErrInvalidName):
		writeValidationError(w, "displayName", err.Error())
	default:
		writeInternalError(w, err)
	}
}
