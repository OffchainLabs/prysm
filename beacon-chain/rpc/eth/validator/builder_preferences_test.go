package validator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	blockchainTesting "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

// The produce-block body is a bare BuilderEntry array; malformed entries are
// ignored per beacon-APIs #630 rather than failing the request.
func TestParseBuilderPreferencesBody(t *testing.T) {
	validEntry := `{"url":"https://b","auth":{"message":{"data":"0x0102","slot":"7"},"signature":"0x` + repeatHex(96) + `"},"max_execution_payment":"1000","min_bid":"5","builder_boost_factor":"120"}`

	t.Run("valid entry parses with all fields", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`[`+validEntry+`]`))
		prefs, err := parseBuilderPreferencesBody(r)
		require.NoError(t, err)
		require.Equal(t, 1, len(prefs))
		p := prefs[0]
		require.Equal(t, "https://b", p.Url)
		require.Equal(t, uint64(1000), uint64(p.Request.Preferences.MaxExecutionPayment))
		require.Equal(t, uint64(5), uint64(*p.MinBid))
		require.Equal(t, uint64(120), *p.BuilderBoostFactor)
		require.DeepEqual(t, []byte{1, 2}, p.Request.Auth.Message.Data)
	})
	t.Run("malformed entries are skipped, valid ones kept", func(t *testing.T) {
		body := `[{"url":""},{"url":"https://no-auth"},{"url":"https://bad","auth":{"message":{"data":"nothex","slot":"1"},"signature":"0x00"}},` + validEntry + `]`
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
		prefs, err := parseBuilderPreferencesBody(r)
		require.NoError(t, err)
		require.Equal(t, 1, len(prefs))
		require.Equal(t, "https://b", prefs[0].Url)
	})
	t.Run("pubkey is parsed and bad pubkeys are skipped", func(t *testing.T) {
		withKey := `{"url":"https://k","auth":{"message":{"data":"0x0102","slot":"7"},"signature":"0x` + repeatHex(96) + `"},"builder_pubkey":"0x` + repeatHex(48) + `"}`
		badKey := `{"url":"https://short","auth":{"message":{"data":"0x0102","slot":"7"},"signature":"0x` + repeatHex(96) + `"},"builder_pubkey":"0x0102"}`
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`[`+withKey+`,`+badKey+`]`))
		prefs, err := parseBuilderPreferencesBody(r)
		require.NoError(t, err)
		require.Equal(t, 1, len(prefs))
		require.Equal(t, "https://k", prefs[0].Url)
		require.Equal(t, 48, len(prefs[0].Pubkey))
	})
	t.Run("empty body yields no preferences", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", bytes.NewBuffer(nil))
		prefs, err := parseBuilderPreferencesBody(r)
		require.NoError(t, err)
		require.Equal(t, 0, len(prefs))
	})
	t.Run("non-array body errors", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"builder_preferences":[]}`))
		_, err := parseBuilderPreferencesBody(r)
		require.NotNil(t, err)
	})
}

func repeatHex(n int) string {
	out := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		out = append(out, '0', '9')
	}
	return string(out)
}

// Two entries sharing a url invalidate the whole request, unlike malformed entries.
// beacon-APIs #630 dropped the unique-url rule on the produce body; duplicates are kept.
func TestParseBuilderPreferencesBody_DuplicateURLAllowed(t *testing.T) {
	entry := `{"url":"https://b","auth":{"message":{"data":"0x01","slot":"7"},"signature":"0x` + repeatHex(96) + `"}}`
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`[`+entry+`,`+entry+`]`))
	prefs, err := parseBuilderPreferencesBody(r)
	require.NoError(t, err)
	require.Equal(t, 2, len(prefs))
}

// An octet-stream body is decoded as SSZ, and the sentinels round-trip back to the
// same effective preferences the JSON path produces.
func TestParseBuilderPreferencesBody_SSZ(t *testing.T) {
	boost := uint64(200)
	minBid := primitives.Gwei(5)
	pubkey := make([]byte, 48)
	pubkey[0] = 0xAB
	prefs := []*eth.BuilderPreferenceV1{{
		Url: "https://b.example",
		Request: &eth.BuilderPreferencesRequestV1{
			Preferences: &eth.BuilderPreferencesV1{MaxExecutionPayment: 1000},
			Auth:        &eth.SignedRequestAuthV1{Message: &eth.RequestAuthV1{Data: []byte{1}, Slot: 7}, Signature: make([]byte, 96)},
		},
		MinBid:             &minBid,
		BuilderBoostFactor: &boost,
		Pubkey:             pubkey,
	}}
	body, err := eth.BuilderPreferencesToSSZ(prefs).MarshalSSZ()
	require.NoError(t, err)

	r := httptest.NewRequest("POST", "/", bytes.NewBuffer(body))
	r.Header.Set("Content-Type", api.OctetStreamMediaType)
	got, err := parseBuilderPreferencesBody(r)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, "https://b.example", got[0].Url)
	require.Equal(t, primitives.Gwei(5), got[0].GetMinBid())
	require.Equal(t, uint64(200), got[0].GetBuilderBoostFactor())
	require.DeepEqual(t, pubkey, got[0].Pubkey)
	require.Equal(t, primitives.Gwei(1000), got[0].Request.Preferences.GetMaxExecutionPayment())
}

// Duplicate urls in an SSZ body are kept, matching the JSON path.
func TestParseBuilderPreferencesBody_SSZ_DuplicateURLAllowed(t *testing.T) {
	entry := func() *eth.BuilderPreferenceV1 {
		return &eth.BuilderPreferenceV1{
			Url: "https://dup.example",
			Request: &eth.BuilderPreferencesRequestV1{
				Preferences: &eth.BuilderPreferencesV1{},
				Auth:        &eth.SignedRequestAuthV1{Message: &eth.RequestAuthV1{Data: []byte{1}}, Signature: make([]byte, 96)},
			},
		}
	}
	body, err := eth.BuilderPreferencesToSSZ([]*eth.BuilderPreferenceV1{entry(), entry()}).MarshalSSZ()
	require.NoError(t, err)
	r := httptest.NewRequest("POST", "/", bytes.NewBuffer(body))
	r.Header.Set("Content-Type", api.OctetStreamMediaType)
	prefs, err := parseBuilderPreferencesBody(r)
	require.NoError(t, err)
	require.Equal(t, 2, len(prefs))
}

func TestSubmitBuilderPreferencesHandler(t *testing.T) {
	pubkey := "0x" + repeatHex(48)
	entry := func(url string) string {
		return `{"url":"` + url + `","auth":{"message":{"data":"0x01","slot":"7"},"signature":"0x` + repeatHex(96) + `"},"max_execution_payment":"1000"}`
	}
	newServer := func(t *testing.T, v1alpha1 eth.BeaconNodeValidatorServer) *Server {
		return &Server{
			V1Alpha1Server:        v1alpha1,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &blockchainTesting.ChainService{},
		}
	}
	post := func(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://foo.example/eth/v1/validator/builder_preferences/"+pubkey, bytes.NewBufferString(body))
		r.SetPathValue("pubkey", pubkey)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		s.SubmitBuilderPreferences(w, r)
		return w
	}

	t.Run("all entries forwarded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		v1alpha1 := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1.EXPECT().SubmitBuilderPreferences(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
		w := post(t, newServer(t, v1alpha1), `[`+entry("https://a")+`,`+entry("https://b")+`]`)
		assert.Equal(t, http.StatusOK, w.Code)
	})
	t.Run("failures reported by index as IndexedErrorMessage", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		v1alpha1 := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		gomock.InOrder(
			v1alpha1.EXPECT().SubmitBuilderPreferences(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom")),
			v1alpha1.EXPECT().SubmitBuilderPreferences(gomock.Any(), gomock.Any()).Return(nil, nil),
		)
		w := post(t, newServer(t, v1alpha1), `[`+entry("https://a")+`,`+entry("https://b")+`]`)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp struct {
			Code     int    `json:"code"`
			Message  string `json:"message"`
			Failures []struct {
				Index   int    `json:"index"`
				Message string `json:"message"`
			} `json:"failures"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, http.StatusBadRequest, resp.Code)
		require.Equal(t, 1, len(resp.Failures))
		require.Equal(t, 0, resp.Failures[0].Index)
		require.StringContains(t, "boom", resp.Failures[0].Message)
	})
	t.Run("malformed entry fails its own index only", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		v1alpha1 := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1.EXPECT().SubmitBuilderPreferences(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
		w := post(t, newServer(t, v1alpha1), `[{"url":""},`+entry("https://b")+`]`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("empty body is a 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		w := post(t, newServer(t, mock2.NewMockBeaconNodeValidatorServer(ctrl)), `[]`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
