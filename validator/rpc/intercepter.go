package rpc

import (
	"context"
	"net/http"
	"strings"

	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthTokenInterceptor is a gRPC unary interceptor to authorize incoming requests.
func (s *Server) AuthTokenInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
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
		// if it's not initialize or has a web prefix
		if (strings.Contains(r.URL.Path, api.WebApiUrlPrefix) || strings.Contains(r.URL.Path, api.KeymanagerApiPrefix)) && !strings.Contains(r.URL.Path, api.SystemLogsPrefix) {
			// ignore some routes
			reqToken := r.Header.Get("Authorization")
			if reqToken == "" {
				httputil.HandleError(w, "Unauthorized: no Authorization header passed. Please use an Authorization header with the jwt created in the prysm wallet", http.StatusUnauthorized)
				return
			}
			tokenParts := strings.Split(reqToken, "Bearer ")
			if len(tokenParts) != 2 {
				httputil.HandleError(w, "Invalid token format", http.StatusBadRequest)
				return
			}

			token := tokenParts[1]
			if strings.TrimSpace(token) != s.authToken || strings.TrimSpace(s.authToken) == "" {
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
	if !ok {
		return status.Errorf(codes.Unauthenticated, "Authorization token could not be found")
	}
	if len(authHeader) < 1 || !strings.Contains(authHeader[0], "Bearer ") {
		return status.Error(codes.Unauthenticated, "Invalid auth header, needs Bearer {token}")
	}
	token := strings.Split(authHeader[0], "Bearer ")[1]
	if strings.TrimSpace(token) != s.authToken || strings.TrimSpace(s.authToken) == "" {
		return status.Errorf(codes.Unauthenticated, "Forbidden: token value is invalid")
	}
	return nil
}
