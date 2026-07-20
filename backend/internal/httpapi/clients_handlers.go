package httpapi

import (
	"net/http"

	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/pagination"
)

func (s *Server) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	var req client.CreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	c, err := s.clients.Create(r.Context(), req)
	if err != nil {
		writeClientError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, client.ToResponse(c))
}

func (s *Server) handleListClients(w http.ResponseWriter, r *http.Request) {
	page, ok := parsePage(w, r)
	if !ok {
		return
	}
	result, err := s.clients.List(r.Context(), page)
	if err != nil {
		writeClientError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	c, err := s.clients.GetByID(r.Context(), id)
	if err != nil {
		writeClientError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, client.ToResponse(c))
}

func (s *Server) handleUpdateClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	fields, err := client.ParseUpdateFields(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	c, err := s.clients.Update(r.Context(), id, fields)
	if err != nil {
		writeClientError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, client.ToResponse(c))
}

func (s *Server) handleDeleteClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := s.clients.Delete(r.Context(), id); err != nil {
		writeClientError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeClientError(w http.ResponseWriter, err error) {
	switch {
	case isErr(err, client.ErrNotFound):
		writeError(w, http.StatusNotFound, "client not found")
	case isErr(err, client.ErrDuplicateName):
		writeError(w, http.StatusConflict, "a client with this name already exists")
	case isErr(err, client.ErrInvalidName):
		writeValidationError(w, "name", err.Error())
	case isErr(err, client.ErrInvalidEmail):
		writeValidationError(w, "email", err.Error())
	case isErr(err, pagination.ErrInvalidCursor):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeInternalError(w, err)
	}
}

// writeInternalError never leaks the underlying error message in the
// response body — only the caller's structured logging (if any) sees it.
func writeInternalError(w http.ResponseWriter, _ error) {
	writeError(w, http.StatusInternalServerError, "internal error")
}
