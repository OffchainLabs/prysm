package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestServer_AuthTokenInterceptor_Verify(t *testing.T) {
	token := "cool-token"
	s := Server{
		authToken: token,
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, nil
	}
	ctxMD := map[string][]string{
		"authorization": {"Bearer " + token},
	}
	ctx := context.Background()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.NoError(t, err)
}

func TestServer_AuthTokenInterceptor_BadToken(t *testing.T) {
	s := Server{
		authToken: "cool-token",
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, nil
	}

	ctxMD := map[string][]string{
		"authorization": {"Bearer bad-token"},
	}
	ctx := context.Background()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.ErrorContains(t, "token value is invalid", err)
}

func TestServer_AuthTokenHandler(t *testing.T) {
	token := "cool-token"

	s := &Server{authToken: token}
	testHandler := s.AuthTokenHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Your test handler logic here
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("Test Response"))
		require.NoError(t, err)
	}))
	t.Run("no auth token was sent", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/eth/v1/keystores", http.NoBody)
		require.NoError(t, err)
		testHandler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		errJson := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), errJson))
		require.StringContains(t, "Unauthorized", errJson.Message)
	})
	t.Run("wrong auth token was sent", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/eth/v1/keystores", http.NoBody)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer YOUR_JWT_TOKEN") // Replace with a valid JWT token
		testHandler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusForbidden, rr.Code)
		errJson := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), errJson))
		require.StringContains(t, "token value is invalid", errJson.Message)
	})
	t.Run("good auth token was sent", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/eth/v1/keystores", http.NoBody)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token) // Replace with a valid JWT token
		testHandler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	})
	t.Run("web endpoint needs auth token", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/api/v2/validator/beacon/status", http.NoBody)
		require.NoError(t, err)
		testHandler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		errJson := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), errJson))
		require.StringContains(t, "Unauthorized", errJson.Message)
	})
	t.Run("initialize does not need auth", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, api.WebUrlPrefix+"initialize", http.NoBody)
		require.NoError(t, err)
		testHandler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	})
	t.Run("health does not need auth", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, api.WebUrlPrefix+"health/logs", http.NoBody)
		require.NoError(t, err)
		testHandler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	})
}
