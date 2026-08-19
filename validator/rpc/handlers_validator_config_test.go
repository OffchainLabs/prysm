package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/derived"
	mocks "github.com/OffchainLabs/prysm/v7/validator/testing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

// setupConfigServer builds a Server with a derived keymanager holding numKeys
// recovered accounts, and returns the server plus the known validating pubkeys.
// The builders endpoints require a gloas-scheduled network, so one is configured.
func setupConfigServer(t *testing.T, numKeys int) (*Server, [][48]byte) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 100
	params.OverrideBeaconConfig(cfg)
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
	// The CRUD tests exercise POST-then-GET round-trips, so the mocked service
	// emulates the real store: reads return the stored settings, writes swap them.
	vs, ok := srv.validatorService.(*validatormock.MockValidatorService)
	require.Equal(t, true, ok)
	var stored *proposer.Settings
	vs.EXPECT().ProposerSettings().DoAndReturn(func() *proposer.Settings {
		return stored
	}).AnyTimes()
	vs.EXPECT().SetProposerSettings(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, s *proposer.Settings) error {
		stored = s
		return nil
	}).AnyTimes()
	vs.EXPECT().UpdateProposerSettings(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, mutate func(*proposer.Settings) (*proposer.Settings, error)) error {
		next, err := mutate(stored.Clone())
		if err != nil {
			return err
		}
		if next != nil {
			stored = next
		}
		return nil
	}).AnyTimes()
	return srv, keys
}

func postBuilderConfig(t *testing.T, s *Server, pubkey, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/validator/"+pubkey+"/builder_config", bytes.NewBufferString(body))
	req.SetPathValue("pubkey", pubkey)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SetBuilderConfig(w, req)
	return w
}

func getBuilderConfig(t *testing.T, s *Server, pubkey string) (*httptest.ResponseRecorder, *BuilderConfig) {
	req := httptest.NewRequest(http.MethodGet, "/eth/v1/validator/"+pubkey+"/builder_config", nil)
	req.SetPathValue("pubkey", pubkey)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.GetBuilderConfig(w, req)
	cfg := &BuilderConfig{}
	if w.Code == http.StatusOK {
		resp := &GetBuilderConfigResponse{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		cfg = resp.Data
	}
	return w, cfg
}

func deleteBuilderConfig(t *testing.T, s *Server, pubkey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/eth/v1/validator/"+pubkey+"/builder_config", nil)
	req.SetPathValue("pubkey", pubkey)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.DeleteBuilderConfig(w, req)
	return w
}

func TestServer_SetBuilderConfig(t *testing.T) {
	t.Run("round trip via GET", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		body := `{"min_bid":"5","builder_boost_factor":"120",` +
			`"builders":[{"url":"https://b.example","auth_data":"0x0102","max_execution_payment":"1000"}]}`

		w := postBuilderConfig(t, srv, pk, body)
		require.Equal(t, http.StatusAccepted, w.Code)

		_, cfg := getBuilderConfig(t, srv, pk)
		require.Equal(t, "5", *cfg.MinBid)
		require.Equal(t, "120", *cfg.BuilderBoostFactor)
		require.Equal(t, 1, len(cfg.Builders))
		require.Equal(t, "https://b.example", cfg.Builders[0].Url)
		require.Equal(t, "1000", *cfg.Builders[0].MaxExecutionPayment)
		// Resolved: the entry omitted min_bid/boost, so GET fills them from the defaults.
		require.Equal(t, "5", *cfg.Builders[0].MinBid)
		require.Equal(t, "120", *cfg.Builders[0].BuilderBoostFactor)
	})

	t.Run("full replace", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])

		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk,
			`{"builders":[{"url":"https://a.example"},{"url":"https://b.example"}]}`).Code)
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk,
			`{"builders":[{"url":"https://c.example"}]}`).Code)

		_, cfg := getBuilderConfig(t, srv, pk)
		require.Equal(t, 1, len(cfg.Builders))
		require.Equal(t, "https://c.example", cfg.Builders[0].Url)
	})

	t.Run("empty array is use-none", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk, `{"builders":[]}`).Code)

		_, cfg := getBuilderConfig(t, srv, pk)
		require.NotNil(t, cfg.Builders)
		require.Equal(t, 0, len(cfg.Builders))
	})

	t.Run("preserves another key's use-none marker", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 2)
		pkA := hexutil.Encode(keys[0][:])
		pkB := hexutil.Encode(keys[1][:])
		require.NoError(t, srv.validatorService.SetProposerSettings(t.Context(), &proposer.Settings{
			Version: proposer.SchemaV2,
			DefaultConfig: &proposer.Option{
				BuilderConfig: &proposer.BuilderConfig{Builders: []*proposer.BuilderEntry{{URL: "https://default.example"}}},
			},
		}))
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pkA, `{"builders":[]}`).Code)
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pkB, `{"builders":[{"url":"https://b.example"}]}`).Code)

		_, cfg := getBuilderConfig(t, srv, pkA)
		require.Equal(t, 0, len(cfg.Builders))
	})

	t.Run("empty object is override-free", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		recipient := common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3")
		require.NoError(t, srv.validatorService.SetProposerSettings(t.Context(), &proposer.Settings{
			Version: proposer.SchemaV1,
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &proposer.BuilderConfig{Enabled: true},
			},
		}))
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk, `{}`).Code)

		// Nothing is stored for the key and the enabled default still applies.
		require.IsNil(t, srv.validatorService.ProposerSettings().ProposeConfig[keys[0]].BuilderConfig)
		_, _, enabled := srv.validatorService.ProposerSettings().RegistrationFor(keys[0])
		require.Equal(t, true, enabled)
	})

	t.Run("gloas-only preferences keep the key's pre-fork registration", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		recipient := common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3")
		require.NoError(t, srv.validatorService.SetProposerSettings(t.Context(), &proposer.Settings{
			Version: proposer.SchemaV1,
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{FeeRecipient: recipient},
				BuilderConfig:      &proposer.BuilderConfig{Enabled: true},
			},
		}))
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk, `{"min_bid":"5"}`).Code)

		// The POST expressed no registration choice, so the enabled default still applies.
		_, _, enabled := srv.validatorService.ProposerSettings().RegistrationFor(keys[0])
		require.Equal(t, true, enabled)

		// An explicit empty list is a registration choice: it opts the key out.
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk, `{"builders":[]}`).Code)
		_, _, enabled = srv.validatorService.ProposerSettings().RegistrationFor(keys[0])
		require.Equal(t, false, enabled)
	})

	t.Run("upgrades v1 settings in place", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		require.NoError(t, srv.validatorService.SetProposerSettings(t.Context(), &proposer.Settings{
			Version: proposer.SchemaV1,
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				keys[0]: {BuilderConfig: &proposer.BuilderConfig{Enabled: true, GasLimit: 999}},
			},
		}))
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk, `{"builders":[{"url":"https://a.example"}]}`).Code)

		got := srv.validatorService.ProposerSettings()
		// Builder lists are v2 content: POST migrates the schema in place.
		require.Equal(t, proposer.SchemaV2, got.Version)
		opt := got.ProposeConfig[keys[0]]
		// The v1 builder gas limit is dropped with the rest of the v1 builder content.
		require.Equal(t, validator.Uint64(0), opt.GasLimit)
		require.Equal(t, 1, len(opt.BuilderConfig.Builders))
	})

	t.Run("builder_pubkeys ride along as the entry's response filter", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		bpk := "0x" + strings.Repeat("ab", 48)
		bpk2 := "0x" + strings.Repeat("cd", 48)
		body := `{"builders":[{"url":"https://a.example","builder_pubkeys":["` + bpk + `","` + bpk2 + `"]},{"url":"https://b.example"}]}`

		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk, body).Code)
		_, cfg := getBuilderConfig(t, srv, pk)
		require.Equal(t, 2, len(cfg.Builders))
		require.Equal(t, "https://a.example", cfg.Builders[0].Url)
		require.DeepEqual(t, []string{bpk, bpk2}, cfg.Builders[0].BuilderPubkeys)
		// Omitted builder_pubkeys resolves to the empty list.
		require.NotNil(t, cfg.Builders[1].BuilderPubkeys)
		require.Equal(t, 0, len(cfg.Builders[1].BuilderPubkeys))
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		bpk := "0x" + strings.Repeat("ab", 48)

		longURL := "https://a.example/" + strings.Repeat("x", 2048)
		cases := map[string]struct {
			body, contains string
		}{
			"entry without url":                                 {`{"builders":[{"min_bid":"1"}]}`, "url is required"},
			"pubkey-only entry":                                 {`{"builders":[{"builder_pubkeys":["` + bpk + `"]}]}`, "url is required"},
			"same url and auth_data":                            {`{"builders":[{"url":"https://a"},{"url":"https://a"}]}`, "share the same url and auth_data"},
			"omitted auth_data collides with its derived value": {`{"builders":[{"url":"https://a"},{"url":"https://a","auth_data":"` + hexutil.Encode([]byte("https://a")) + `"}]}`, "share the same url and auth_data"},
			"invalid url":                                       {`{"builders":[{"url":"not a url"}]}`, "url is not a valid URL"},
			"url too long":                                      {`{"builders":[{"url":"` + longURL + `"}]}`, "url exceeds 2048 bytes"},
			"invalid builder_pubkeys entry":                     {`{"builders":[{"url":"https://a","builder_pubkeys":["0x1234"]}]}`, "builder_pubkeys contains an invalid BLS public key"},
			"invalid auth_data hex":                             {`{"builders":[{"url":"https://a","auth_data":"0xzz"}]}`, "auth_data is not valid hex"},
			"auth_data too long":                                {`{"builders":[{"url":"https://a","auth_data":"0x` + strings.Repeat("ab", 4097) + `"}]}`, "auth_data must be 1 to 4096 bytes"},
			"auth_data empty":                                   {`{"builders":[{"url":"https://a","auth_data":"0x"}]}`, "auth_data must be 1 to 4096 bytes"},
			"too many builder_pubkeys":                          {`{"builders":[{"url":"https://a","builder_pubkeys":[` + strings.TrimSuffix(strings.Repeat(`"`+bpk+`",`, 65), ",") + `]}]}`, "builder_pubkeys exceeds 64 keys"},
			"non-numeric min_bid":                               {`{"min_bid":"abc","builders":[]}`, "min_bid is not a valid uint64"},
			"null entry":                                        {`{"builders":[null]}`, "builders[0] is null"},
			"non-numeric entry max":                             {`{"builders":[{"url":"https://a"},{"url":"https://b","max_execution_payment":"x"}]}`, "builders[1].max_execution_payment is not a valid uint64"},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				w := postBuilderConfig(t, srv, pk, tc.body)
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Equal(t, true, strings.Contains(w.Body.String(), tc.contains), "body: %s", w.Body.String())
			})
		}
	})

	t.Run("same url with distinct auth_data is a legal pair", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		body := `{"builders":[{"url":"https://a","auth_data":"0x01"},{"url":"https://a","auth_data":"0x02"}]}`
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk, body).Code)
		_, cfg := getBuilderConfig(t, srv, pk)
		require.Equal(t, 2, len(cfg.Builders))
	})

	t.Run("max entries", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		entries := make([]string, 0, maxBuilderEntries+1)
		for i := 0; i <= maxBuilderEntries; i++ {
			entries = append(entries, `{"url":"https://b`+strconv.Itoa(i)+`.example"}`)
		}
		body := `{"builders":[` + strings.Join(entries, ",") + `]}`
		w := postBuilderConfig(t, srv, pk, body)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, true, strings.Contains(w.Body.String(), "exceeds 64 entries"))
	})

	t.Run("validator service nil", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		srv.validatorService = nil
		w := postBuilderConfig(t, srv, hexutil.Encode(keys[0][:]), `{"builders":[]}`)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

func TestServer_GetBuilderConfig(t *testing.T) {
	// GET is fully resolved: omitted auth_data becomes the url's UTF-8 bytes, and
	// unset values become the runtime fallbacks (no floor, neutral boost, trustless-only).
	t.Run("nil proposer settings resolve to runtime defaults", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		// No settings were ever created: the nil-receiver chain must still
		// produce a fully resolved response rather than panic or error.
		w, cfg := getBuilderConfig(t, srv, pk)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "0", *cfg.MinBid)
		require.Equal(t, "100", *cfg.BuilderBoostFactor)
		require.NotNil(t, cfg.Builders)
		require.Equal(t, 0, len(cfg.Builders))
	})

	t.Run("resolves omitted values", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk,
			`{"builders":[{"url":"https://a.example"}]}`).Code)
		_, cfg := getBuilderConfig(t, srv, pk)
		require.Equal(t, "0", *cfg.MinBid)
		require.Equal(t, "100", *cfg.BuilderBoostFactor)
		require.Equal(t, 1, len(cfg.Builders))
		require.Equal(t, hexutil.Encode([]byte("https://a.example")), *cfg.Builders[0].AuthData)
		require.Equal(t, "0", *cfg.Builders[0].MinBid)
		require.Equal(t, "100", *cfg.Builders[0].BuilderBoostFactor)
		require.Equal(t, "0", *cfg.Builders[0].MaxExecutionPayment)
	})

	t.Run("resolves default_config for an unconfigured key", func(t *testing.T) {
		srv, keys := setupConfigServer(t, 1)
		pk := hexutil.Encode(keys[0][:])
		require.NoError(t, srv.validatorService.SetProposerSettings(t.Context(), &proposer.Settings{
			Version: proposer.SchemaV2,
			DefaultConfig: &proposer.Option{
				BuilderConfig: &proposer.BuilderConfig{Builders: []*proposer.BuilderEntry{{URL: "https://default.example"}}},
			},
		}))

		_, cfg := getBuilderConfig(t, srv, pk)
		require.Equal(t, 1, len(cfg.Builders))
		require.Equal(t, "https://default.example", cfg.Builders[0].Url)
	})
}

func TestServer_BuilderConfig_RequireGloasScheduled(t *testing.T) {
	// The default test config has no gloas fork epoch: all three endpoints refuse.
	srv := &Server{}
	pk := "0x" + strings.Repeat("ab", 48)
	for name, w := range map[string]*httptest.ResponseRecorder{
		"get":    getBuilderConfigRecorder(srv, pk),
		"post":   postBuilderConfig(t, srv, pk, `{"builders":[]}`),
		"delete": deleteBuilderConfig(t, srv, pk),
	} {
		require.Equal(t, http.StatusNotImplemented, w.Code, "endpoint: %s", name)
		require.Equal(t, true, strings.Contains(w.Body.String(), "gloas fork scheduled"), "endpoint: %s", name)
	}
}

func getBuilderConfigRecorder(s *Server, pubkey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/eth/v1/validator/"+pubkey+"/builder_config", nil)
	req.SetPathValue("pubkey", pubkey)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.GetBuilderConfig(w, req)
	return w
}

func TestServer_DeleteBuilderConfig(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])

	// Removing an absent configuration succeeds.
	require.Equal(t, http.StatusNoContent, deleteBuilderConfig(t, srv, pk).Code)

	require.Equal(t, http.StatusAccepted, postBuilderConfig(t, srv, pk, `{"builders":[{"url":"https://a"}]}`).Code)
	require.Equal(t, http.StatusNoContent, deleteBuilderConfig(t, srv, pk).Code)

	// After delete the key follows defaults (no per-key builders).
	_, cfg := getBuilderConfig(t, srv, pk)
	require.Equal(t, 0, len(cfg.Builders))
}
