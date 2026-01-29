package rpc

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const bearerPrefix = "Bearer "

// AuthTokenInterceptor is a gRPC unary interceptor to authorize incoming requests.
func (s *Server) AuthTokenInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if err := s.authorize(ctx); err != nil {
			return nil, err
		}
		h, err := handler(ctx, req)
		log.WithError(err).WithFields(logrus.Fields{
			"FullMethod": info.FullMethod,
			"Server":     info.Server,
		}).Debug("Request handled")
		return h, err
	}
}

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

// Authorize the token received is valid.
func (s *Server) authorize(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "Retrieving metadata failed")
	}

	authHeader, ok := md["authorization"]
	if !ok || len(authHeader) == 0 {
		return status.Error(codes.Unauthenticated, "Authorization token could not be found")
	}

	token, ok := strings.CutPrefix(authHeader[0], bearerPrefix)
	if !ok {
		return status.Error(codes.Unauthenticated, "Invalid auth header, needs Bearer {token}")
	}

	token = strings.TrimSpace(token)
	if token == "" ||
		len(s.authToken) == 0 ||
		len(token) != len(s.authToken) ||
		subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) != 1 {
		return status.Error(codes.Unauthenticated, "Forbidden: token value is invalid")
	}

	return nil
}
