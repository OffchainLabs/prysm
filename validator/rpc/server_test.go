package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestServer_InitializeRoutes(t *testing.T) {
	s := Server{
		router: http.NewServeMux(),
	}
	err := s.InitializeRoutes()
	require.NoError(t, err)

	wantRouteList := map[string][]string{
		"/eth/v1/keystores":                          {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/eth/v1/remotekeys":                         {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/eth/v1/validator/{pubkey}/gas_limit":       {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/eth/v1/validator/{pubkey}/feerecipient":    {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/eth/v1/validator/{pubkey}/voluntary_exit":  {http.MethodPost},
		"/eth/v1/validator/{pubkey}/graffiti":        {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/v2/validator/health/version":               {http.MethodGet},
		"/v2/validator/health/logs/validator/stream": {http.MethodGet},
		"/v2/validator/health/logs/beacon/stream":    {http.MethodGet},
		"/v2/validator/wallet":                       {http.MethodGet},
		"/v2/validator/wallet/create":                {http.MethodPost},
		"/v2/validator/wallet/keystores/validate":    {http.MethodPost},
		"/v2/validator/wallet/recover":               {http.MethodPost},
		"/v2/validator/slashing-protection/export":   {http.MethodGet},
		"/v2/validator/slashing-protection/import":   {http.MethodPost},
		"/v2/validator/accounts":                     {http.MethodGet},
		"/v2/validator/accounts/backup":              {http.MethodPost},
		"/v2/validator/accounts/voluntary-exit":      {http.MethodPost},
		"/v2/validator/beacon/balances":              {http.MethodGet},
		"/v2/validator/beacon/peers":                 {http.MethodGet},
		"/v2/validator/beacon/status":                {http.MethodGet},
		"/v2/validator/beacon/summary":               {http.MethodGet},
		"/v2/validator/beacon/validators":            {http.MethodGet},
		"/v2/validator/initialize":                   {http.MethodGet},
	}
	for route, methods := range wantRouteList {
		for _, method := range methods {
			r, err := http.NewRequest(method, route, nil)
			require.NoError(t, err)
			if method == http.MethodGet {
				_, path := s.router.Handler(r)
				require.Equal(t, "GET "+route, path)
			} else if method == http.MethodPost {
				_, path := s.router.Handler(r)
				require.Equal(t, "POST "+route, path)
			} else if method == http.MethodDelete {
				_, path := s.router.Handler(r)
				require.Equal(t, "DELETE "+route, path)
			} else {
				t.Errorf("Unsupported method %v", method)
			}
		}
	}
}

func TestServer_InitializeRoutesWithWebHandler(t *testing.T) {
	s := Server{
		router:    http.NewServeMux(),
		walletDir: t.TempDir(),
	}
	require.NoError(t, s.InitializeRoutesWithWebHandler())

	serve := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	requireNotFound := func(t *testing.T, w *httptest.ResponseRecorder) {
		require.Equal(t, http.StatusNotFound, w.Code)
		e := &httputil.DefaultJsonError{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), e))
		require.Equal(t, http.StatusNotFound, e.Code)
	}

	t.Run("unknown path", func(t *testing.T) {
		requireNotFound(t, serve("/eth/v1/nonsense"))
	})
	t.Run("unknown api path", func(t *testing.T) {
		requireNotFound(t, serve("/api/v2/validator/nonsense"))
	})
	t.Run("known path", func(t *testing.T) {
		require.Equal(t, http.StatusOK, serve("/v2/validator/initialize").Code)
	})
	t.Run("known api path", func(t *testing.T) {
		require.Equal(t, http.StatusOK, serve("/api/v2/validator/initialize").Code)
	})
}
