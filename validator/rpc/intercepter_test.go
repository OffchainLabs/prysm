package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestServer_AuthTokenInterceptor(t *testing.T) {
	token := "cool-token"
	interceptor := (&Server{authToken: token}).AuthTokenInterceptor()

	tests := []struct {
		name            string
		metadata        metadata.MD
		handlerErr      error
		wantHandlerCall bool
		wantCode        codes.Code
		wantErrSubstr   string
	}{
		{
			name:            "calls handler with valid token",
			metadata:        metadata.MD{"authorization": {"Bearer " + token}},
			wantHandlerCall: true,
		},
		{
			name:            "propagates handler error after successful auth",
			metadata:        metadata.MD{"authorization": {"Bearer " + token}},
			handlerErr:      status.Error(codes.Internal, "handler failure"),
			wantHandlerCall: true,
			wantCode:        codes.Internal,
			wantErrSubstr:   "handler failure",
		},
		{
			name:          "rejects request before handler on invalid token",
			metadata:      metadata.MD{"authorization": {"Bearer bad-token"}},
			wantCode:      codes.Unauthenticated,
			wantErrSubstr: "token value is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.metadata != nil {
				ctx = metadata.NewIncomingContext(ctx, tt.metadata)
			}

			handlerCalled := false
			_, err := interceptor(ctx, "xyz", &grpc.UnaryServerInfo{FullMethod: "Proto.CreateWallet"}, func(ctx context.Context, req any) (any, error) {
				handlerCalled = true
				return nil, tt.handlerErr
			})

			require.Equal(t, tt.wantHandlerCall, handlerCalled)
			if tt.wantErrSubstr == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, tt.wantErrSubstr, err)
			require.Equal(t, tt.wantCode, status.Code(err))
		})
	}
}

func TestServer_authorize(t *testing.T) {
	token := "cool-token"
	server := &Server{authToken: token}

	tests := []struct {
		name          string
		metadata      metadata.MD
		wantCode      codes.Code
		wantErrSubstr string
	}{
		{
			name:          "returns invalid argument when metadata is missing",
			wantCode:      codes.InvalidArgument,
			wantErrSubstr: "Retrieving metadata failed",
		},
		{
			name:          "returns unauthenticated when authorization header is missing",
			metadata:      metadata.MD{"other-header": {"some-value"}},
			wantCode:      codes.Unauthenticated,
			wantErrSubstr: "Authorization token could not be found",
		},
		{
			name:          "returns unauthenticated for malformed bearer prefix",
			metadata:      metadata.MD{"authorization": {"Bearercool-token"}},
			wantCode:      codes.Unauthenticated,
			wantErrSubstr: "Invalid auth header",
		},
		{
			name:          "returns unauthenticated for empty bearer token",
			metadata:      metadata.MD{"authorization": {"Bearer "}},
			wantCode:      codes.Unauthenticated,
			wantErrSubstr: "token value is invalid",
		},
		{
			name:          "returns unauthenticated for invalid token value",
			metadata:      metadata.MD{"authorization": {"Bearer bad-token"}},
			wantCode:      codes.Unauthenticated,
			wantErrSubstr: "token value is invalid",
		},
		{
			name:     "accepts matching bearer token",
			metadata: metadata.MD{"authorization": {"Bearer " + token}},
		},
		{
			name:     "accepts token with surrounding whitespace",
			metadata: metadata.MD{"authorization": {"Bearer  " + token + "  "}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.metadata != nil {
				ctx = metadata.NewIncomingContext(ctx, tt.metadata)
			}

			err := server.authorize(ctx)
			if tt.wantErrSubstr == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, tt.wantErrSubstr, err)
			require.Equal(t, tt.wantCode, status.Code(err))
		})
	}
}

func TestServer_AuthTokenHandler_ProtectsRoutes(t *testing.T) {
	token := "cool-token"
	handler := (&Server{authToken: token}).AuthTokenHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name          string
		path          string
		authHeader    string
		wantCode      int
		wantErrSubstr string
	}{
		{
			name:          "rejects missing token on keymanager endpoint",
			path:          "/eth/v1/keystores",
			wantCode:      http.StatusUnauthorized,
			wantErrSubstr: "Unauthorized",
		},
		{
			name:       "accepts matching token on keymanager endpoint",
			path:       "/eth/v1/keystores",
			authHeader: "Bearer " + token,
			wantCode:   http.StatusOK,
		},
		{
			name:          "requires token on web api endpoint",
			path:          "/api/v2/validator/beacon/status",
			wantCode:      http.StatusUnauthorized,
			wantErrSubstr: "Unauthorized",
		},
		{
			name:          "requires token on direct web endpoint",
			path:          "/v2/validator/beacon/status",
			wantCode:      http.StatusUnauthorized,
			wantErrSubstr: "Unauthorized",
		},
		{
			name:     "allows initialize without auth",
			path:     api.WebUrlPrefix + "initialize",
			wantCode: http.StatusOK,
		},
		{
			name:     "allows health without auth",
			path:     api.WebUrlPrefix + "health/logs",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := newAuthTestRequest(t, tt.path)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			handler.ServeHTTP(rr, req)

			require.Equal(t, tt.wantCode, rr.Code)
			if tt.wantErrSubstr != "" {
				requireAuthErrorMessage(t, rr, tt.wantErrSubstr)
			}
		})
	}
}

func TestServer_AuthTokenHandler_ValidatesAuthorizationHeader(t *testing.T) {
	token := "cool-token"
	handler := (&Server{authToken: token}).AuthTokenHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name          string
		authHeader    string
		wantCode      int
		wantErrSubstr string
	}{
		{
			name:          "rejects malformed bearer prefix",
			authHeader:    "Bearertoken",
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "Invalid token format",
		},
		{
			name:          "rejects empty bearer token",
			authHeader:    "Bearer ",
			wantCode:      http.StatusForbidden,
			wantErrSubstr: "token value is invalid",
		},
		{
			name:          "rejects invalid token value",
			authHeader:    "Bearer bad-token",
			wantCode:      http.StatusForbidden,
			wantErrSubstr: "token value is invalid",
		},
		{
			name:       "accepts matching token",
			authHeader: "Bearer " + token,
			wantCode:   http.StatusOK,
		},
		{
			name:       "accepts token with surrounding whitespace",
			authHeader: "Bearer  " + token + "  ",
			wantCode:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := newAuthTestRequest(t, "/eth/v1/keystores")
			req.Header.Set("Authorization", tt.authHeader)

			handler.ServeHTTP(rr, req)

			require.Equal(t, tt.wantCode, rr.Code)
			if tt.wantErrSubstr != "" {
				requireAuthErrorMessage(t, rr, tt.wantErrSubstr)
			}
		})
	}
}

func BenchmarkServer_AuthTokenHandler(b *testing.B) {
	token := "cool-token"
	handler := (&Server{authToken: token}).AuthTokenHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, err := http.NewRequest(http.MethodGet, "/eth/v1/keystores", http.NoBody)
	require.NoError(b, err)
	req.Header.Set("Authorization", "Bearer "+token)

	b.ReportAllocs()
	for b.Loop() {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(b, http.StatusOK, rr.Code)
	}
}

func BenchmarkServer_AuthTokenInterceptor(b *testing.B) {
	token := "cool-token"
	interceptor := (&Server{authToken: token}).AuthTokenInterceptor()
	unaryInfo := &grpc.UnaryServerInfo{FullMethod: "Proto.CreateWallet"}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{"authorization": {"Bearer " + token}})

	b.ReportAllocs()
	for b.Loop() {
		_, err := interceptor(ctx, "xyz", unaryInfo, func(ctx context.Context, req any) (any, error) {
			return nil, nil
		})
		require.NoError(b, err)
	}
}

func newAuthTestRequest(t *testing.T, path string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, path, http.NoBody)
	require.NoError(t, err)
	return req
}

func requireAuthErrorMessage(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()

	errJSON := &httputil.DefaultJsonError{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), errJSON))
	require.StringContains(t, want, errJSON.Message)
}
