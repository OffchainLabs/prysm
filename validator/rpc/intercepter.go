package rpc

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
)

const bearerPrefix = "Bearer "

// AuthTokenHandler is an HTTP handler to authorize a route.
func (s *Server) AuthTokenHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		needsAuth := strings.Contains(path, api.WebApiUrlPrefix) || strings.Contains(path, api.KeymanagerApiPrefix)
		// Protect direct (non-/api) web endpoints too; otherwise callers can bypass auth by hitting /v2/validator/*.
		if strings.HasPrefix(path, api.WebUrlPrefix) &&
			!strings.HasPrefix(path, api.WebUrlPrefix+"initialize") &&
			!strings.HasPrefix(path, api.WebUrlPrefix+"health/") {
			needsAuth = true
		}

		if needsAuth && !strings.Contains(path, api.SystemLogsPrefix) {
			// ignore some routes
			reqToken := r.Header.Get("Authorization")
			if reqToken == "" {
				httputil.HandleError(w, "Unauthorized: no Authorization header passed. Please use an Authorization header with the jwt created in the prysm wallet", http.StatusUnauthorized)
				return
			}

			token, ok := strings.CutPrefix(reqToken, bearerPrefix)
			if !ok {
				httputil.HandleError(w, "Invalid token format", http.StatusBadRequest)
				return
			}

			token = strings.TrimSpace(token)
			if token == "" ||
				len(s.authToken) == 0 ||
				len(token) != len(s.authToken) ||
				subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) != 1 {
				httputil.HandleError(w, "Forbidden: token value is invalid", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
