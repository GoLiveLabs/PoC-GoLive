package httpapi

import (
	"encoding/json"
	"net/http"

	"live-orchestrator/backend/internal/orchestrator"
)

type previewRequest struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func (s *Server) handleGetLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.orch.LiveState())
}

func (s *Server) handleSetPreview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	var (
		st  orchestrator.LiveState
		err error
	)
	switch req.Kind {
	case string(orchestrator.LiveKindCamera):
		st, err = s.orch.SetPreviewCamera(req.ID)
	case string(orchestrator.LiveKindScene):
		st, err = s.orch.SetPreviewScene(req.ID)
	default:
		writeError(w, http.StatusUnprocessableEntity, "kind inválido")
		return
	}
	if err != nil {
		writeLiveError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleCut(w http.ResponseWriter, r *http.Request) {
	st, err := s.orch.Cut(r.Context())
	if err != nil {
		writeLiveError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func writeLiveError(w http.ResponseWriter, err error) {
	switch {
	case isErr(err, orchestrator.ErrCameraNotFound):
		writeError(w, http.StatusNotFound, "câmera não encontrada")
	case isErr(err, orchestrator.ErrSceneNotFound):
		writeError(w, http.StatusNotFound, "cena não encontrada")
	case isErr(err, orchestrator.ErrPreviewEmpty):
		writeError(w, http.StatusConflict, "nada em prévia")
	case isErr(err, orchestrator.ErrSourceUnavailable):
		writeError(w, http.StatusConflict, "fonte indisponível")
	case isErr(err, orchestrator.ErrCameraOffline):
		writeError(w, http.StatusConflict, "câmera está offline")
	case isErr(err, orchestrator.ErrOBSUnreachable):
		writeError(w, http.StatusBadGateway, "OBS está inacessível no momento")
	case isErr(err, orchestrator.ErrPositionOBSInputMissing):
		writeError(w, http.StatusBadGateway, "input do OBS da posição não encontrado")
	default:
		writeError(w, http.StatusInternalServerError, "erro interno")
	}
}
