package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/derived"
	mocks "github.com/OffchainLabs/prysm/v7/validator/testing"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// setupConfigServer builds a Server with a derived keymanager holding numKeys
// recovered accounts, and returns the server plus the known validating pubkeys.
func setupConfigServer(t *testing.T, numKeys int) (*Server, [][48]byte) {
	ctx := t.Context()
	srv := setupServerWithWallet(t)
	km, err := srv.validatorService.Keymanager()
	require.NoError(t, err)
	dr, ok := km.(*derived.Keymanager)
	require.Equal(t, true, ok)
	require.NoError(t, dr.RecoverAccountsFromMnemonic(ctx, mocks.TestMnemonic, derived.DefaultMnemonicLanguage, "", numKeys))
	keys, err := dr.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	require.Equal(t, numKeys, len(keys))
	return srv, keys
}

func postBuilders(t *testing.T, s *Server, pubkey, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/validator/"+pubkey+"/builders", bytes.NewBufferString(body))
	req.SetPathValue("pubkey", pubkey)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SetBuilders(w, req)
	return w
}

func getBuilders(t *testing.T, s *Server, pubkey string) (*httptest.ResponseRecorder, *BuilderConfigJson) {
	req := httptest.NewRequest(http.MethodGet, "/eth/v1/validator/"+pubkey+"/builders", nil)
	req.SetPathValue("pubkey", pubkey)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.GetBuilders(w, req)
	cfg := &BuilderConfigJson{}
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), cfg))
	}
	return w, cfg
}

func deleteBuilders(t *testing.T, s *Server, pubkey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/eth/v1/validator/"+pubkey+"/builders", nil)
	req.SetPathValue("pubkey", pubkey)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.DeleteBuilders(w, req)
	return w
}

func TestServer_SetGetBuilders_RoundTrip(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])
	body := `{"enabled":true,"min_bid":"5","builder_boost_factor":"120",` +
		`"builders":[{"url":"https://b.example","auth_data":"0x0102","max_execution_payment":"1000"}]}`

	w := postBuilders(t, srv, pk, body)
	require.Equal(t, http.StatusAccepted, w.Code)

	_, cfg := getBuilders(t, srv, pk)
	require.NotNil(t, cfg.Enabled)
	require.Equal(t, true, *cfg.Enabled)
	require.Equal(t, "5", *cfg.MinBid)
	require.Equal(t, "120", *cfg.BuilderBoostFactor)
	require.Equal(t, 1, len(cfg.Builders))
	require.Equal(t, "https://b.example", *cfg.Builders[0].Url)
	require.Equal(t, "1000", *cfg.Builders[0].MaxExecutionPayment)
	// Resolved: the entry omitted min_bid/boost, so GET fills them from the defaults.
	require.Equal(t, "5", *cfg.Builders[0].MinBid)
	require.Equal(t, "120", *cfg.Builders[0].BuilderBoostFactor)
}

func TestServer_SetBuilders_FullReplace(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])

	require.Equal(t, http.StatusAccepted, postBuilders(t, srv, pk,
		`{"enabled":true,"builders":[{"url":"https://a.example"},{"url":"https://b.example"}]}`).Code)
	require.Equal(t, http.StatusAccepted, postBuilders(t, srv, pk,
		`{"enabled":true,"builders":[{"url":"https://c.example"}]}`).Code)

	_, cfg := getBuilders(t, srv, pk)
	require.Equal(t, 1, len(cfg.Builders))
	require.Equal(t, "https://c.example", *cfg.Builders[0].Url)
}

func TestServer_SetBuilders_EmptyArrayIsUseNone(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])
	require.Equal(t, http.StatusAccepted, postBuilders(t, srv, pk, `{"enabled":true,"builders":[]}`).Code)

	_, cfg := getBuilders(t, srv, pk)
	require.NotNil(t, cfg.Builders)
	require.Equal(t, 0, len(cfg.Builders))
}

func TestServer_SetBuilders_BuilderPubkeyOnlyEntry(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])
	bpk := "0x" + strings.Repeat("ab", 48)
	body := `{"enabled":true,"builders":[{"builder_pubkey":"` + bpk + `","min_bid":"9"}]}`

	require.Equal(t, http.StatusAccepted, postBuilders(t, srv, pk, body).Code)
	_, cfg := getBuilders(t, srv, pk)
	require.Equal(t, 1, len(cfg.Builders))
	require.IsNil(t, cfg.Builders[0].Url)
	require.Equal(t, bpk, *cfg.Builders[0].BuilderPubkey)
}

func TestServer_SetBuilders_Rejects(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])
	bpk := "0x" + strings.Repeat("ab", 48)

	cases := map[string]struct {
		body, contains string
	}{
		"missing enabled":        {`{"builders":[{"url":"https://a"}]}`, "enabled is required"},
		"entry has neither":      {`{"enabled":true,"builders":[{"min_bid":"1"}]}`, "at least one of url and builder_pubkey"},
		"same url and auth_data": {`{"enabled":true,"builders":[{"url":"https://a"},{"url":"https://a"}]}`, "share the same url and auth_data"},
		"p2p pubkey repeated":    {`{"enabled":true,"builders":[{"builder_pubkey":"` + bpk + `"},{"builder_pubkey":"` + bpk + `"}]}`, "two p2p-policy entries share the same builder_pubkey"},
		"invalid url":            {`{"enabled":true,"builders":[{"url":"not a url"}]}`, "url is not a valid URL"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := postBuilders(t, srv, pk, tc.body)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Equal(t, true, strings.Contains(w.Body.String(), tc.contains), "body: %s", w.Body.String())
		})
	}
}

// Same url with distinct auth_data is a legal pair, not a duplicate.
func TestServer_SetBuilders_SharedURLDistinctAuth(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])
	body := `{"enabled":true,"builders":[{"url":"https://a","auth_data":"0x01"},{"url":"https://a","auth_data":"0x02"}]}`
	require.Equal(t, http.StatusAccepted, postBuilders(t, srv, pk, body).Code)
	_, cfg := getBuilders(t, srv, pk)
	require.Equal(t, 2, len(cfg.Builders))
}

func TestServer_SetBuilders_MaxEntries(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])
	entries := make([]string, 0, maxBuilderEntries+1)
	for i := 0; i <= maxBuilderEntries; i++ {
		entries = append(entries, `{"url":"https://b`+strconv.Itoa(i)+`.example"}`)
	}
	body := `{"enabled":true,"builders":[` + strings.Join(entries, ",") + `]}`
	w := postBuilders(t, srv, pk, body)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, true, strings.Contains(w.Body.String(), "exceeds 64 entries"))
}

func TestServer_DeleteBuilders(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])

	// Nothing configured yet -> 404.
	require.Equal(t, http.StatusNotFound, deleteBuilders(t, srv, pk).Code)

	require.Equal(t, http.StatusAccepted, postBuilders(t, srv, pk, `{"enabled":true,"builders":[{"url":"https://a"}]}`).Code)
	require.Equal(t, http.StatusNoContent, deleteBuilders(t, srv, pk).Code)

	// After delete the key follows defaults (no per-key builders).
	_, cfg := getBuilders(t, srv, pk)
	require.NotNil(t, cfg.Enabled)
	require.Equal(t, false, *cfg.Enabled)
	require.Equal(t, 0, len(cfg.Builders))
}

// GET on an unconfigured key resolves against default_config.
func TestServer_GetBuilders_ResolvesDefault(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])
	require.NoError(t, srv.validatorService.SetProposerSettings(t.Context(), &proposer.Settings{
		Version: proposer.SchemaV2,
		DefaultConfig: &proposer.Option{
			BuilderConfig: &proposer.BuilderConfig{Enabled: true, Builders: []*proposer.BuilderEntry{{URL: "https://default.example"}}},
		},
	}))

	_, cfg := getBuilders(t, srv, pk)
	require.NotNil(t, cfg.Enabled)
	require.Equal(t, true, *cfg.Enabled)
	require.Equal(t, 1, len(cfg.Builders))
	require.Equal(t, "https://default.example", *cfg.Builders[0].Url)
}

func TestServer_SetBuilders_ValidatorServiceNil(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	srv.validatorService = nil
	w := postBuilders(t, srv, hexutil.Encode(keys[0][:]), `{"enabled":true}`)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}
