package rpc

import (
	"net/http"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
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
