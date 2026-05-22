package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func validProposerPreferencesBody() string {
	return `[{
		"message": {
			"dependent_root": "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			"proposal_slot": "32",
			"validator_index": "2",
			"fee_recipient": "0x0000000000000000000000000000000000000000",
			"target_gas_limit": "30000000"
		},
		"signature": "0x` + strings.Repeat("00", 96) + `"
	}]`
}

func TestSubmitSignedProposerPreferences_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().
		SubmitSignedProposerPreferences(gomock.Any(), gomock.AssignableToTypeOf(&eth.SubmitSignedProposerPreferencesRequest{})).
		DoAndReturn(func(_ context.Context, req *eth.SubmitSignedProposerPreferencesRequest) (*emptypb.Empty, error) {
			require.Equal(t, 1, len(req.SignedProposerPreferences))
			require.Equal(t, uint64(30_000_000), req.SignedProposerPreferences[0].Message.TargetGasLimit)
			return &emptypb.Empty{}, nil
		})

	s := &Server{V1Alpha1Server: v1alpha1Server}
	req := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/proposer_preferences", bytes.NewBufferString(validProposerPreferencesBody()))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.SubmitSignedProposerPreferences(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSubmitSignedProposerPreferences_NoBody(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.SubmitSignedProposerPreferences(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	e := &httputil.DefaultJsonError{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), e))
	assert.Equal(t, true, strings.Contains(e.Message, "No data submitted"))
}

func TestSubmitSignedProposerPreferences_Empty(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "http://example.com", bytes.NewBufferString("[]"))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.SubmitSignedProposerPreferences(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitSignedProposerPreferences_InvalidJSON(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "http://example.com", bytes.NewBufferString(`[{"message": null, "signature": "0x00"}]`))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.SubmitSignedProposerPreferences(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func runWithGRPCError(t *testing.T, code codes.Code) int {
	t.Helper()
	ctrl := gomock.NewController(t)
	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().
		SubmitSignedProposerPreferences(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(code, "boom"))

	s := &Server{V1Alpha1Server: v1alpha1Server}
	req := httptest.NewRequest(http.MethodPost, "http://example.com", bytes.NewBufferString(validProposerPreferencesBody()))
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.SubmitSignedProposerPreferences(w, req)
	return w.Code
}

func TestSubmitSignedProposerPreferences_InvalidArgumentMapsTo400(t *testing.T) {
	assert.Equal(t, http.StatusBadRequest, runWithGRPCError(t, codes.InvalidArgument))
}

func TestSubmitSignedProposerPreferences_UnavailableMapsTo503(t *testing.T) {
	assert.Equal(t, http.StatusServiceUnavailable, runWithGRPCError(t, codes.Unavailable))
}

func TestSubmitSignedProposerPreferences_InternalMapsTo500(t *testing.T) {
	assert.Equal(t, http.StatusInternalServerError, runWithGRPCError(t, codes.Internal))
}
