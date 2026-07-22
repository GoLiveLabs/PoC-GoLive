package httpapi

import (
	"encoding/json"
	"net/http"

	"live-orchestrator/backend/internal/orchestrator"
)

type createSceneRequest struct {
	Name        string   `json:"name"`
	PositionIDs []string `json:"positionIds"`
}

type patchSceneRequest struct {
	Name        *string   `json:"name"`
	PositionIDs *[]string `json:"positionIds"`
}

func (s *Server) handleListScenes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.orch.Scenes())
}

func (s *Server) handleCreateScene(w http.ResponseWriter, r *http.Request) {
	var req createSceneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}
	sc, err := s.orch.CreateScene(req.Name, req.PositionIDs)
	if err == nil {
		writeJSON(w, http.StatusCreated, sc)
		return
	}
	writeSceneError(w, err)
}

func (s *Server) handlePatchScene(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req patchSceneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	var (
		sc  orchestrator.Scene
		err error
	)
	if req.Name != nil {
		sc, err = s.orch.RenameScene(id, *req.Name)
		if err != nil {
			writeSceneError(w, err)
			return
		}
	}
	if req.PositionIDs != nil {
		sc, err = s.orch.UpdateScenePositions(id, *req.PositionIDs)
		if err != nil {
			writeSceneError(w, err)
			return
		}
	}
	if req.Name == nil && req.PositionIDs == nil {
		// No fields: return current scene if it exists by renaming to itself via list.
		for _, existing := range s.orch.Scenes() {
			if existing.ID == id {
				writeJSON(w, http.StatusOK, existing)
				return
			}
		}
		writeSceneError(w, orchestrator.ErrSceneNotFound)
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

func (s *Server) handleDeleteScene(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.orch.DeleteScene(id); err != nil {
		writeSceneError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeSceneError(w http.ResponseWriter, err error) {
	switch {
	case isErr(err, orchestrator.ErrSceneNotFound):
		writeError(w, http.StatusNotFound, "cena não encontrada")
	case isErr(err, orchestrator.ErrPositionNotFound):
		writeError(w, http.StatusNotFound, "posição não encontrada")
	case isErr(err, orchestrator.ErrSceneNameRequired):
		writeError(w, http.StatusUnprocessableEntity, "nome de cena é obrigatório")
	case isErr(err, orchestrator.ErrSceneNameTaken):
		writeError(w, http.StatusUnprocessableEntity, "nome de cena já utilizado")
	case isErr(err, orchestrator.ErrReservedPosition):
		writeError(w, http.StatusUnprocessableEntity, "posição reservada")
	case isErr(err, orchestrator.ErrSceneIsLive):
		writeError(w, http.StatusConflict, "cena está ao vivo")
	default:
		writeError(w, http.StatusInternalServerError, "erro interno")
	}
}
