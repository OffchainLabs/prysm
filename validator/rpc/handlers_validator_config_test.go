package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/derived"
	mocks "github.com/OffchainLabs/prysm/v7/validator/testing"
	"github.com/ethereum/go-ethereum/common"
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

func postValidatorConfig(t *testing.T, s *Server, rawBody string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/validator/config", bytes.NewBufferString(rawBody))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SetValidatorConfig(w, req)
	return w
}

func getValidatorConfig(t *testing.T, s *Server, query string) *GetValidatorConfigResponse {
	req := httptest.NewRequest(http.MethodGet, "/eth/v1/validator/config"+query, nil)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.GetValidatorConfig(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	resp := &GetValidatorConfigResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), resp))
	return resp
}

func TestServer_SetGetValidatorConfig_FullDocument(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	// Seed a read-only default_config so GET surfaces it alongside the per-key fragment.
	require.NoError(t, srv.validatorService.SetProposerSettings(t.Context(), &proposer.Settings{
		Version:       proposer.SchemaV2,
		DefaultConfig: &proposer.Option{FeeRecipientConfig: &proposer.FeeRecipientConfig{FeeRecipient: common.HexToAddress("0x1111111111111111111111111111111111111111")}},
	}))

	pk := hexutil.Encode(keys[0][:])
	feeRecipient := "0xabcf8e0d4e9587369b2301d0790347320302cc09"
	body := `{"configs":{"` + pk + `":{` +
		`"fee_recipient":"` + feeRecipient + `",` +
		`"target_gas_limit":"30000000",` +
		`"graffiti":"hello world",` +
		`"builder":{"enabled":true,"builders":[{"url":"http://relay.example"}],` +
		`"max_execution_payment":"1000000","min_bid":"5","builder_boost_factor":"150"}` +
		`}}}`

	w := postValidatorConfig(t, srv, body)
	require.Equal(t, http.StatusOK, w.Code)
	setResp := &SetValidatorConfigResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), setResp))
	require.NotNil(t, setResp.Data[pk])
	require.Equal(t, configStatusSet, setResp.Data[pk].Status)

	getResp := getValidatorConfig(t, srv, "")
	require.NotNil(t, getResp.Data.DefaultConfig)
	require.NotNil(t, getResp.Data.DefaultConfig.FeeRecipient)
	require.Equal(t, common.HexToAddress("0x1111111111111111111111111111111111111111"), common.HexToAddress(*getResp.Data.DefaultConfig.FeeRecipient))

	cfg := getResp.Data.Configs[pk]
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.FeeRecipient)
	require.Equal(t, common.HexToAddress(feeRecipient), common.HexToAddress(*cfg.FeeRecipient))
	require.NotNil(t, cfg.TargetGasLimit)
	require.Equal(t, "30000000", *cfg.TargetGasLimit)
	require.NotNil(t, cfg.Graffiti)
	require.Equal(t, "hello world", *cfg.Graffiti)
	require.NotNil(t, cfg.Builder)
	require.NotNil(t, cfg.Builder.Enabled)
	require.Equal(t, true, *cfg.Builder.Enabled)
	require.Equal(t, 1, len(cfg.Builder.Builders))
	require.Equal(t, "http://relay.example", cfg.Builder.Builders[0].Url)
	require.NotNil(t, cfg.Builder.MaxExecutionPayment)
	require.Equal(t, "1000000", *cfg.Builder.MaxExecutionPayment)
	require.NotNil(t, cfg.Builder.MinBid)
	require.Equal(t, "5", *cfg.Builder.MinBid)
	require.NotNil(t, cfg.Builder.BuilderBoostFactor)
	require.Equal(t, "150", *cfg.Builder.BuilderBoostFactor)
}

func TestServer_SetValidatorConfig_FullReplacementNotMerge(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])
	feeRecipient := "0xabcf8e0d4e9587369b2301d0790347320302cc09"

	w := postValidatorConfig(t, srv, `{"configs":{"`+pk+`":{"fee_recipient":"`+feeRecipient+`","graffiti":"first"}}}`)
	require.Equal(t, http.StatusOK, w.Code)
	first := getValidatorConfig(t, srv, "")
	require.NotNil(t, first.Data.Configs[pk].Graffiti)

	// Re-POST with only fee_recipient; the whole fragment is replaced, so graffiti disappears.
	w = postValidatorConfig(t, srv, `{"configs":{"`+pk+`":{"fee_recipient":"`+feeRecipient+`"}}}`)
	require.Equal(t, http.StatusOK, w.Code)
	setResp := &SetValidatorConfigResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), setResp))
	require.Equal(t, configStatusSet, setResp.Data[pk].Status)

	second := getValidatorConfig(t, srv, "")
	cfg := second.Data.Configs[pk]
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.FeeRecipient)
	require.IsNil(t, cfg.Graffiti)
}

func TestServer_SetValidatorConfig_EmptyDocumentClearsKey(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])

	w := postValidatorConfig(t, srv, `{"configs":{"`+pk+`":{"graffiti":"present"}}}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, getValidatorConfig(t, srv, "").Data.Configs[pk])

	w = postValidatorConfig(t, srv, `{"configs":{"`+pk+`":{}}}`)
	require.Equal(t, http.StatusOK, w.Code)
	setResp := &SetValidatorConfigResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), setResp))
	require.Equal(t, configStatusSet, setResp.Data[pk].Status)

	_, present := getValidatorConfig(t, srv, "").Data.Configs[pk]
	require.Equal(t, false, present)
}

func TestServer_SetValidatorConfig_UnknownPubkeyNotFound(t *testing.T) {
	srv, _ := setupConfigServer(t, 1)
	// 48-byte pubkey never recovered into the keymanager.
	unknown := "0x" + "abcdef0011223344556677889900aabbccddeeff00112233445566778899aabbccddeeff001122334455667788990011"

	w := postValidatorConfig(t, srv, `{"configs":{"`+unknown+`":{"graffiti":"x"}}}`)
	require.Equal(t, http.StatusOK, w.Code)
	setResp := &SetValidatorConfigResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), setResp))
	require.NotNil(t, setResp.Data[unknown])
	require.Equal(t, configStatusNotFound, setResp.Data[unknown].Status)

	// Settings are untouched: no per-key config was written.
	getResp := getValidatorConfig(t, srv, "")
	require.Equal(t, 0, len(getResp.Data.Configs))
}

func TestServer_SetValidatorConfig_UnknownJSONFieldRejected(t *testing.T) {
	srv, keys := setupConfigServer(t, 1)
	pk := hexutil.Encode(keys[0][:])

	// Unknown field in the per-key config document.
	w := postValidatorConfig(t, srv, `{"configs":{"`+pk+`":{"not_a_field":"x"}}}`)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Unknown field in the top-level envelope.
	w = postValidatorConfig(t, srv, `{"configs":{},"bogus":true}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestServer_SetValidatorConfig_PerKeyErrorsIsolated(t *testing.T) {
	srv, keys := setupConfigServer(t, 2)
	good := hexutil.Encode(keys[0][:])
	badFee := hexutil.Encode(keys[1][:])

	body := `{"configs":{` +
		`"` + good + `":{"graffiti":"ok"},` +
		`"` + badFee + `":{"fee_recipient":"0xnothex"}` +
		`}}`
	w := postValidatorConfig(t, srv, body)
	require.Equal(t, http.StatusOK, w.Code)
	setResp := &SetValidatorConfigResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), setResp))

	require.Equal(t, configStatusSet, setResp.Data[good].Status)
	require.Equal(t, configStatusError, setResp.Data[badFee].Status)
	require.NotEqual(t, "", setResp.Data[badFee].Message)

	getResp := getValidatorConfig(t, srv, "")
	require.NotNil(t, getResp.Data.Configs[good])
	_, badPresent := getResp.Data.Configs[badFee]
	require.Equal(t, false, badPresent)

	// A non-uint target_gas_limit is likewise a per-key error.
	w = postValidatorConfig(t, srv, `{"configs":{"`+good+`":{"target_gas_limit":"notanumber"}}}`)
	require.Equal(t, http.StatusOK, w.Code)
	setResp = &SetValidatorConfigResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), setResp))
	require.Equal(t, configStatusError, setResp.Data[good].Status)
	require.NotEqual(t, "", setResp.Data[good].Message)
}

func TestServer_GetValidatorConfig_PubkeyFilter(t *testing.T) {
	srv, keys := setupConfigServer(t, 2)
	pk0 := hexutil.Encode(keys[0][:])
	pk1 := hexutil.Encode(keys[1][:])

	w := postValidatorConfig(t, srv, `{"configs":{"`+pk0+`":{"graffiti":"a"},"`+pk1+`":{"graffiti":"b"}}}`)
	require.Equal(t, http.StatusOK, w.Code)

	all := getValidatorConfig(t, srv, "")
	require.Equal(t, 2, len(all.Data.Configs))

	filtered := getValidatorConfig(t, srv, "?pubkeys="+pk0)
	require.Equal(t, 1, len(filtered.Data.Configs))
	require.NotNil(t, filtered.Data.Configs[pk0])
	_, present := filtered.Data.Configs[pk1]
	require.Equal(t, false, present)
}

func TestServer_SetValidatorConfig_ValidatorServiceNil(t *testing.T) {
	s := &Server{}
	w := postValidatorConfig(t, s, `{"configs":{}}`)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
