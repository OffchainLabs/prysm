package beacon_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

func testSignedProposerPreferences() *ethpb.SignedProposerPreferences {
	return &ethpb.SignedProposerPreferences{
		Message: &ethpb.ProposerPreferences{
			DependentRoot:  bytes.Repeat([]byte{0xcc}, 32),
			ProposalSlot:   32,
			ValidatorIndex: 2,
			FeeRecipient:   bytes.Repeat([]byte{0xab}, 20),
			TargetGasLimit: 30_000_000,
		},
		Signature: bytes.Repeat([]byte{0x01}, 96),
	}
}

func TestSubmitSignedProposerPreferences_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pref := testSignedProposerPreferences()
	sszBody, err := pref.MarshalSSZ()
	require.NoError(t, err)

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		proposerPreferencesEndpoint,
		map[string]string{"Eth-Consensus-Version": "gloas"},
		bytes.NewBuffer(sszBody),
	).Return(nil, nil, nil).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	require.NoError(t, client.submitSignedProposerPreferences(t.Context(), []*ethpb.SignedProposerPreferences{pref}))
}

func TestSubmitSignedProposerPreferences_FallsBackToJSONOn415(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pref := testSignedProposerPreferences()
	jsonBody, err := json.Marshal([]*structs.SignedProposerPreferences{structs.SignedProposerPreferencesFromConsensus(pref)})
	require.NoError(t, err)

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		proposerPreferencesEndpoint,
		gomock.Any(),
		gomock.Any(),
	).Return(nil, nil, &httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType, Message: "unsupported media type"}).Times(1)
	handler.EXPECT().Post(
		gomock.Any(),
		proposerPreferencesEndpoint,
		map[string]string{"Eth-Consensus-Version": "gloas"},
		bytes.NewBuffer(jsonBody),
		nil,
	).Return(nil).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	require.NoError(t, client.submitSignedProposerPreferences(t.Context(), []*ethpb.SignedProposerPreferences{pref}))
}

func TestSubmitSignedProposerPreferences_NonMediaTypeErrorNoFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		proposerPreferencesEndpoint,
		map[string]string{"Eth-Consensus-Version": "gloas"},
		gomock.Any(),
	).Return(nil, nil, errors.New("foo error")).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	err := client.submitSignedProposerPreferences(t.Context(), []*ethpb.SignedProposerPreferences{testSignedProposerPreferences()})
	assert.ErrorContains(t, "foo error", err)
}

func TestSubmitSignedProposerPreferences_NilEntry(t *testing.T) {
	client := &beaconApiValidatorClient{}
	err := client.submitSignedProposerPreferences(t.Context(), []*ethpb.SignedProposerPreferences{nil})
	assert.ErrorContains(t, "is nil", err)
}
