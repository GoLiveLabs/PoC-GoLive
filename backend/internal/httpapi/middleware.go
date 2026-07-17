package httpapi

import (
	"errors"
	"net/http"
)

func isErr(err, target error) bool {
	return errors.Is(err, target)
}

// devFrontendOrigin is the Angular dev server origin, liberated for CORS
// per the plan's "modo dev" requirement.
const devFrontendOrigin = "http://localhost:4200"

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", devFrontendOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Token")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authMiddleware requires a valid X-Api-Token header (or "api_token" query
// param, since the browser WebSocket API cannot set custom headers) on every
// route except /health.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get("X-Api-Token")
		if token == "" {
			token = r.URL.Query().Get("api_token")
		}
		if token == "" || token != s.token {
			writeError(w, http.StatusUnauthorized, "token de acesso inválido ou ausente")
			return
		}
		next.ServeHTTP(w, r)
	})
}
