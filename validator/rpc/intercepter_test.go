package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

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
			name:     "leaves non-keymanager endpoints unauthenticated",
			path:     "/healthz",
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
