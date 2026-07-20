package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/pagination"
)

// writeValidationError produces a 422 response with a single field->message
// entry, mirroring the shape a validation library would populate.
func writeValidationError(w http.ResponseWriter, field, message string) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":  message,
		"errors": map[string]string{field: message},
	})
}

// decodeJSON reads and decodes a JSON body, writing a 400 response and
// returning false on any decode failure (including an empty body, which
// Decode reports as io.EOF).
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "request body is required")
		} else {
			writeError(w, http.StatusBadRequest, "malformed request body")
		}
		return false
	}
	return true
}

// readBody reads the raw request body, for handlers (client PATCH) that need
// to distinguish an absent key from an explicit null before decoding.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return nil, false
	}
	return b, true
}

// pathUUID parses a path parameter as a UUID, writing a 400 response and
// returning false if it is missing or malformed.
func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	raw := r.PathValue(name)
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+name+" in path")
		return uuid.UUID{}, false
	}
	return id, true
}

// queryUUID parses an optional UUID query parameter. ok is false (with a 400
// already written) only when the param is present but not a valid UUID.
func queryUUID(w http.ResponseWriter, r *http.Request, name string) (id *uuid.UUID, ok bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, true
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+name+" query parameter")
		return nil, false
	}
	return &parsed, true
}

// queryBool parses an optional boolean query parameter ("true"/"false").
func queryBool(w http.ResponseWriter, r *http.Request, name string) (val *bool, ok bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, true
	}
	switch raw {
	case "true":
		v := true
		return &v, true
	case "false":
		v := false
		return &v, true
	default:
		writeError(w, http.StatusBadRequest, "invalid "+name+" query parameter")
		return nil, false
	}
}

// parsePage parses the shared "limit"/"cursor" pagination query params.
func parsePage(w http.ResponseWriter, r *http.Request) (pagination.Request, bool) {
	page, err := pagination.ParseRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return pagination.Request{}, false
	}
	return page, true
}
