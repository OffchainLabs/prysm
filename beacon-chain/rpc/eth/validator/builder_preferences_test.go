package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	ethpbalpha "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func testBuilderPreferencesConsensusRequest() *ethpbalpha.BuilderPreferencesRequestV1 {
	return &ethpbalpha.BuilderPreferencesRequestV1{
		Preferences: &ethpbalpha.BuilderPreferencesV1{MaxExecutionPayment: 1_000_000},
		Auth: &ethpbalpha.SignedRequestAuthV1{
			Message: &ethpbalpha.RequestAuthV1{
				Data: []byte("https://builder.example.com"),
				Slot: 32,
			},
			Signature: bytes.Repeat([]byte{0x01}, 96),
		},
	}
}

func builderPrefsTestPubkey() []byte { return bytes.Repeat([]byte{0xde}, 48) }

func newBuilderPreferencesRequest(t *testing.T, ssz bool) *http.Request {
	t.Helper()
	var body []byte
	if ssz {
		b, err := testBuilderPreferencesConsensusRequest().MarshalSSZ()
		require.NoError(t, err)
		body = b
	} else {
		b, err := json.Marshal(structs.BuilderPreferencesRequestV1FromConsensus(testBuilderPreferencesConsensusRequest()))
		require.NoError(t, err)
		body = b
	}
	req := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/builder_preferences/x", bytes.NewBuffer(body))
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	if ssz {
		req.Header.Set("Content-Type", api.OctetStreamMediaType)
	}
	req.SetPathValue("pubkey", hexutil.Encode(builderPrefsTestPubkey()))
	return req
}

func TestSubmitBuilderPreferences_JSON_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().
		SubmitBuilderPreferences(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *ethpbalpha.SubmitBuilderPreferencesRequest) (*emptypb.Empty, error) {
			assert.DeepEqual(t, builderPrefsTestPubkey(), req.ValidatorPubkey)
			require.NotNil(t, req.Request)
			assert.Equal(t, uint64(1_000_000), uint64(req.Request.Preferences.MaxExecutionPayment))
			return &emptypb.Empty{}, nil
		})

	s := &Server{V1Alpha1Server: v1alpha1Server}
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SubmitBuilderPreferences(w, newBuilderPreferencesRequest(t, false))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSubmitBuilderPreferences_SSZ_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().SubmitBuilderPreferences(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil)

	s := &Server{V1Alpha1Server: v1alpha1Server}
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SubmitBuilderPreferences(w, newBuilderPreferencesRequest(t, true))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSubmitBuilderPreferences_MissingVersionHeader(t *testing.T) {
	s := &Server{}
	req := newBuilderPreferencesRequest(t, false)
	req.Header.Del(api.VersionHeader)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SubmitBuilderPreferences(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitBuilderPreferences_PreGloasVersion(t *testing.T) {
	s := &Server{}
	req := newBuilderPreferencesRequest(t, false)
	req.Header.Set(api.VersionHeader, version.String(version.Fulu))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SubmitBuilderPreferences(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitBuilderPreferences_InvalidPubkey(t *testing.T) {
	s := &Server{}
	req := newBuilderPreferencesRequest(t, false)
	req.SetPathValue("pubkey", "0xnothex")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SubmitBuilderPreferences(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitBuilderPreferences_NoBody(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
	req.Header.Set(api.VersionHeader, version.String(version.Gloas))
	req.SetPathValue("pubkey", hexutil.Encode(builderPrefsTestPubkey()))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SubmitBuilderPreferences(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func runBuilderPrefsGRPCError(t *testing.T, code codes.Code) int {
	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().SubmitBuilderPreferences(gomock.Any(), gomock.Any()).Return(nil, status.Error(code, "boom"))
	s := &Server{V1Alpha1Server: v1alpha1Server}
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	s.SubmitBuilderPreferences(w, newBuilderPreferencesRequest(t, false))
	return w.Code
}

func TestSubmitBuilderPreferences_ErrorMapping(t *testing.T) {
	assert.Equal(t, http.StatusBadRequest, runBuilderPrefsGRPCError(t, codes.InvalidArgument))
	assert.Equal(t, http.StatusServiceUnavailable, runBuilderPrefsGRPCError(t, codes.FailedPrecondition))
	assert.Equal(t, http.StatusServiceUnavailable, runBuilderPrefsGRPCError(t, codes.Unavailable))
	assert.Equal(t, http.StatusInternalServerError, runBuilderPrefsGRPCError(t, codes.Internal))
}
