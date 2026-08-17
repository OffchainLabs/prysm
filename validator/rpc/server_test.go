package rpc

import (
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestServer_InitializeRoutes(t *testing.T) {
	s := Server{
		router: http.NewServeMux(),
	}
	err := s.InitializeRoutes()
	require.NoError(t, err)

	wantRouteList := map[string][]string{
		"/eth/v1/keystores":                         {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/eth/v1/remotekeys":                        {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/eth/v1/validator/{pubkey}/gas_limit":      {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/eth/v1/validator/{pubkey}/feerecipient":   {http.MethodGet, http.MethodPost, http.MethodDelete},
		"/eth/v1/validator/{pubkey}/voluntary_exit": {http.MethodPost},
		"/eth/v1/validator/{pubkey}/graffiti":       {http.MethodGet, http.MethodPost, http.MethodDelete},
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
