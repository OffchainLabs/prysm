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
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}
	ctxMD := map[string][]string{
		"authorization": {"Bearer " + token},
	}
	ctx := t.Context()
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
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}

	ctxMD := map[string][]string{
		"authorization": {"Bearer bad-token"},
	}
	ctx := t.Context()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.ErrorContains(t, "token value is invalid", err)
}

func TestServer_AuthTokenInterceptor_MalformedBearerPrefix(t *testing.T) {
	s := Server{
		authToken: "cool-token",
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}

	ctxMD := map[string][]string{
		"authorization": {"Bearercool-token"}, // Missing space after Bearer
	}
	ctx := t.Context()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.ErrorContains(t, "Invalid auth header", err)
}

func TestServer_AuthTokenInterceptor_EmptyBearerToken(t *testing.T) {
	s := Server{
		authToken: "cool-token",
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}

	ctxMD := map[string][]string{
		"authorization": {"Bearer "}, // Empty token after Bearer
	}
	ctx := t.Context()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.ErrorContains(t, "token value is invalid", err)
}

func TestServer_AuthTokenInterceptor_NoMetadata(t *testing.T) {
	s := Server{
		authToken: "cool-token",
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}

	ctx := t.Context()
	// No metadata attached
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.ErrorContains(t, "Retrieving metadata failed", err)
}

func TestServer_AuthTokenInterceptor_NoAuthorizationHeader(t *testing.T) {
	s := Server{
		authToken: "cool-token",
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}

	ctxMD := map[string][]string{
		"other-header": {"some-value"},
	}
	ctx := t.Context()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.ErrorContains(t, "Authorization token could not be found", err)
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
	t.Run("direct /v2 endpoint also needs auth token (no /api bypass)", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/v2/validator/beacon/status", http.NoBody)
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

func TestServer_AuthTokenHandler_MalformedBearerPrefix(t *testing.T) {
	token := "cool-token"
	s := &Server{authToken: token}

	handler := s.AuthTokenHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/eth/v1/keystores", http.NoBody)
	require.NoError(t, err)

	// Missing space after "Bearer"
	req.Header.Set("Authorization", "Bearertoken")

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	errJson := &httputil.DefaultJsonError{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), errJson))
	require.StringContains(t, "Invalid token format", errJson.Message)
}

func TestServer_AuthTokenHandler_EmptyBearerToken(t *testing.T) {
	token := "cool-token"
	s := &Server{authToken: token}

	handler := s.AuthTokenHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/eth/v1/keystores", http.NoBody)
	require.NoError(t, err)

	// Bearer with empty token
	req.Header.Set("Authorization", "Bearer ")

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)

	errJson := &httputil.DefaultJsonError{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), errJson))
	require.StringContains(t, "token value is invalid", errJson.Message)
}

func TestServer_AuthTokenHandler_TokenWithWhitespace(t *testing.T) {
	token := "cool-token"
	s := &Server{authToken: token}

	handler := s.AuthTokenHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/eth/v1/keystores", http.NoBody)
	require.NoError(t, err)

	// Token with leading/trailing whitespace (should be trimmed and validated)
	req.Header.Set("Authorization", "Bearer  "+token+"  ")

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

// Benchmark tests
func BenchmarkAuthTokenHandler_ValidToken(b *testing.B) {
	token := "cool-token"
	s := &Server{authToken: token}

	handler := s.AuthTokenHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest(http.MethodGet, "/eth/v1/keystores", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkAuthTokenHandler_InvalidToken(b *testing.B) {
	token := "cool-token"
	s := &Server{authToken: token}

	handler := s.AuthTokenHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest(http.MethodGet, "/eth/v1/keystores", http.NoBody)
	req.Header.Set("Authorization", "Bearer bad-token")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkAuthTokenInterceptor_ValidToken(b *testing.B) {
	token := "cool-token"
	s := Server{
		authToken: token,
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}
	ctxMD := map[string][]string{
		"authorization": {"Bearer " + token},
	}
	ctx := context.Background()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	}
}

func BenchmarkAuthTokenInterceptor_InvalidToken(b *testing.B) {
	token := "cool-token"
	s := Server{
		authToken: token,
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}
	ctxMD := map[string][]string{
		"authorization": {"Bearer bad-token"},
	}
	ctx := context.Background()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	}
}
