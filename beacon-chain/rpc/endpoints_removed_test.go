package rpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestRemovedEndpoints(t *testing.T) {
	s := &Service{cfg: &Config{}}

	// Registering every endpoint also guards against pattern conflicts between removed and live
	// routes, since http.ServeMux panics on conflicting patterns.
	mux := http.NewServeMux()
	for _, e := range s.endpoints(true, nil, nil, nil, nil, nil, nil) {
		for _, m := range e.methods {
			mux.HandleFunc(fmt.Sprintf("%s %s", m, e.template), e.handlerWithMiddleware())
		}
	}

	wildcards := strings.NewReplacer("{block_id}", "head", "{state_id}", "head", "{slot}", "1")
	removed := append(s.removedEndpoints(), s.removedDebugEndpoints()...)
	assert.NotEqual(t, 0, len(removed))
	for _, e := range removed {
		for _, m := range e.methods {
			t.Run(m+" "+e.template, func(t *testing.T) {
				request := httptest.NewRequest(m, wildcards.Replace(e.template), nil)
				writer := httptest.NewRecorder()
				mux.ServeHTTP(writer, request)

				require.Equal(t, http.StatusGone, writer.Code)
				errJson := &httputil.DefaultJsonError{}
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), errJson))
				assert.Equal(t, http.StatusGone, errJson.Code)
				assert.StringContains(t, "removed from the Beacon API specification", errJson.Message)
			})
		}
	}
}

func TestRemovedEndpoint_Message(t *testing.T) {
	testCases := []struct {
		name        string
		replacement string
		wantMessage string
	}{
		{
			name:        "with replacement",
			replacement: "/eth/v2/beacon/blocks/{block_id}",
			wantMessage: "Use /eth/v2/beacon/blocks/{block_id} instead.",
		},
		{
			name:        "without replacement",
			replacement: "",
			wantMessage: "It has no replacement.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := removedEndpoint("GetBlock", "/eth/v1/beacon/blocks/{block_id}", []string{http.MethodGet}, tc.replacement)
			request := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/blocks/head", nil)
			writer := httptest.NewRecorder()
			e.handler(writer, request)

			require.Equal(t, http.StatusGone, writer.Code)
			errJson := &httputil.DefaultJsonError{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), errJson))
			assert.StringContains(t, tc.wantMessage, errJson.Message)
		})
	}
}
