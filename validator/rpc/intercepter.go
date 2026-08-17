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
		if strings.HasPrefix(r.URL.Path, api.KeymanagerApiPrefix) {
			reqToken := r.Header.Get("Authorization")
			if reqToken == "" {
				httputil.HandleError(w, "Unauthorized: no Authorization header passed. Please use an Authorization header with the auth token created in the prysm wallet", http.StatusUnauthorized)
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
