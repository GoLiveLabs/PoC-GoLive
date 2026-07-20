package httpapi

import (
	"net/http"

	"live-orchestrator/backend/internal/ingest"
)

func (s *Server) handleCreateIngest(w http.ResponseWriter, r *http.Request) {
	clientID, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	var req ingest.CreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	ing, err := s.ingests.Create(r.Context(), clientID, req)
	if err != nil {
		writeIngestError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ingest.ToResponse(ing))
}

func (s *Server) handleListIngestsByClient(w http.ResponseWriter, r *http.Request) {
	clientID, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	s.listIngests(w, r, ingest.ListFilter{ClientID: &clientID})
}

func (s *Server) handleListIngestsFlat(w http.ResponseWriter, r *http.Request) {
	clientID, ok := queryUUID(w, r, "clientId")
	if !ok {
		return
	}
	s.listIngests(w, r, ingest.ListFilter{ClientID: clientID})
}

func (s *Server) listIngests(w http.ResponseWriter, r *http.Request, filter ingest.ListFilter) {
	isActive, ok := queryBool(w, r, "isActive")
	if !ok {
		return
	}
	filter.IsActive = isActive

	page, ok := parsePage(w, r)
	if !ok {
		return
	}

	result, err := s.ingests.List(r.Context(), filter, page)
	if err != nil {
		writeIngestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetIngest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	ing, err := s.ingests.GetByID(r.Context(), id)
	if err != nil {
		writeIngestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ingest.ToResponse(ing))
}

func (s *Server) handleUpdateIngest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req ingest.UpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	ing, err := s.ingests.Update(r.Context(), id, req)
	if err != nil {
		writeIngestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ingest.ToResponse(ing))
}

func (s *Server) handleDeleteIngest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := s.ingests.Delete(r.Context(), id); err != nil {
		writeIngestError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeIngestError(w http.ResponseWriter, err error) {
	switch {
	case isErr(err, ingest.ErrNotFound):
		writeError(w, http.StatusNotFound, "ingest not found")
	case isErr(err, ingest.ErrClientNotFound):
		writeError(w, http.StatusNotFound, "client not found")
	case isErr(err, ingest.ErrDuplicateURL):
		writeError(w, http.StatusConflict, "an ingest with this url already exists for this client")
	case isErr(err, ingest.ErrURLRequired):
		writeError(w, http.StatusBadRequest, err.Error())
	case isErr(err, ingest.ErrInvalidURL):
		writeValidationError(w, "url", err.Error())
	case isErr(err, ingest.ErrUnsupportedProto):
		writeValidationError(w, "url", err.Error())
	default:
		writeInternalError(w, err)
	}
}
