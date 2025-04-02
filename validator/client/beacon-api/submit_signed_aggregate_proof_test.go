package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	testhelpers "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/test-helpers"
	"go.uber.org/mock/gomock"
)

func TestSubmitSignedAggregateSelectionProof_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProof := generateSignedAggregateAndProofJson()
	marshalledSignedAggregateSignedAndProof, err := json.Marshal([]*structs.SignedAggregateAttestationAndProof{jsonifySignedAggregateAndProof(signedAggregateAndProof)})
	require.NoError(t, err)

	ctx := context.Background()
	headers := map[string]string{"Eth-Consensus-Version": version.String(signedAggregateAndProof.Message.Version())}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProof),
		nil,
	).Return(
		nil,
	).Times(1)

	attestationDataRoot, err := signedAggregateAndProof.Message.Aggregate.Data.HashTreeRoot()
	require.NoError(t, err)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	resp, err := validatorClient.submitSignedAggregateSelectionProof(ctx, &ethpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: signedAggregateAndProof,
	})
	require.NoError(t, err)
	assert.DeepEqual(t, attestationDataRoot[:], resp.AttestationDataRoot)
}

func TestSubmitSignedAggregateSelectionProof_BadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProof := generateSignedAggregateAndProofJson()
	marshalledSignedAggregateSignedAndProof, err := json.Marshal([]*structs.SignedAggregateAttestationAndProof{jsonifySignedAggregateAndProof(signedAggregateAndProof)})
	require.NoError(t, err)

	ctx := context.Background()
	headers := map[string]string{"Eth-Consensus-Version": version.String(signedAggregateAndProof.Message.Version())}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProof),
		nil,
	).Return(
		errors.New("bad request"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err = validatorClient.submitSignedAggregateSelectionProof(ctx, &ethpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: signedAggregateAndProof,
	})
	assert.ErrorContains(t, "bad request", err)
}

func TestSubmitSignedAggregateSelectionProof_Fallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProof := generateSignedAggregateAndProofJson()
	marshalledSignedAggregateSignedAndProof, err := json.Marshal([]*structs.SignedAggregateAttestationAndProof{jsonifySignedAggregateAndProof(signedAggregateAndProof)})
	require.NoError(t, err)

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	headers := map[string]string{"Eth-Consensus-Version": version.String(signedAggregateAndProof.Message.Version())}
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProof),
		nil,
	).Return(
		&httputil.DefaultJsonError{
			Code: http.StatusNotFound,
		},
	).Times(1)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/validator/aggregate_and_proofs",
		nil,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProof),
		nil,
	).Return(
		nil,
	).Times(1)

	attestationDataRoot, err := signedAggregateAndProof.Message.Aggregate.Data.HashTreeRoot()
	require.NoError(t, err)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	resp, err := validatorClient.submitSignedAggregateSelectionProof(ctx, &ethpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: signedAggregateAndProof,
	})
	require.NoError(t, err)
	assert.DeepEqual(t, attestationDataRoot[:], resp.AttestationDataRoot)
}

func TestSubmitSignedAggregateSelectionProofElectra_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProofElectra := generateSignedAggregateAndProofElectraJson()
	marshalledSignedAggregateSignedAndProofElectra, err := json.Marshal([]*structs.SignedAggregateAttestationAndProofElectra{jsonifySignedAggregateAndProofElectra(signedAggregateAndProofElectra)})
	require.NoError(t, err)

	ctx := context.Background()
	headers := map[string]string{"Eth-Consensus-Version": version.String(signedAggregateAndProofElectra.Message.Version())}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProofElectra),
		nil,
	).Return(
		nil,
	).Times(1)

	attestationDataRoot, err := signedAggregateAndProofElectra.Message.Aggregate.Data.HashTreeRoot()
	require.NoError(t, err)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	resp, err := validatorClient.submitSignedAggregateSelectionProofElectra(ctx, &ethpb.SignedAggregateSubmitElectraRequest{
		SignedAggregateAndProof: signedAggregateAndProofElectra,
	})
	require.NoError(t, err)
	assert.DeepEqual(t, attestationDataRoot[:], resp.AttestationDataRoot)
}

func TestSubmitSignedAggregateSelectionProofElectra_BadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProofElectra := generateSignedAggregateAndProofElectraJson()
	marshalledSignedAggregateSignedAndProofElectra, err := json.Marshal([]*structs.SignedAggregateAttestationAndProofElectra{jsonifySignedAggregateAndProofElectra(signedAggregateAndProofElectra)})
	require.NoError(t, err)

	ctx := context.Background()
	headers := map[string]string{"Eth-Consensus-Version": version.String(signedAggregateAndProofElectra.Message.Version())}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProofElectra),
		nil,
	).Return(
		errors.New("bad request"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err = validatorClient.submitSignedAggregateSelectionProofElectra(ctx, &ethpb.SignedAggregateSubmitElectraRequest{
		SignedAggregateAndProof: signedAggregateAndProofElectra,
	})
	assert.ErrorContains(t, "bad request", err)
}

func generateSignedAggregateAndProofJson() *ethpb.SignedAggregateAttestationAndProof {
	return &ethpb.SignedAggregateAttestationAndProof{
		Message: &ethpb.AggregateAttestationAndProof{
			AggregatorIndex: 72,
			Aggregate: &ethpb.Attestation{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Slot:            75,
					CommitteeIndex:  76,
					BeaconBlockRoot: testhelpers.FillByteSlice(32, 38),
					Source: &ethpb.Checkpoint{
						Epoch: 78,
						Root:  testhelpers.FillByteSlice(32, 79),
					},
					Target: &ethpb.Checkpoint{
						Epoch: 80,
						Root:  testhelpers.FillByteSlice(32, 81),
					},
				},
				Signature: testhelpers.FillByteSlice(96, 82),
			},
			SelectionProof: testhelpers.FillByteSlice(96, 82),
		},
		Signature: testhelpers.FillByteSlice(96, 82),
	}
}

func generateSignedAggregateAndProofElectraJson() *ethpb.SignedAggregateAttestationAndProofElectra {
	return &ethpb.SignedAggregateAttestationAndProofElectra{
		Message: &ethpb.AggregateAttestationAndProofElectra{
			AggregatorIndex: 72,
			Aggregate: &ethpb.AttestationElectra{
				AggregationBits: testhelpers.FillByteSlice(4, 74),
				Data: &ethpb.AttestationData{
					Slot:            75,
					CommitteeIndex:  76,
					BeaconBlockRoot: testhelpers.FillByteSlice(32, 38),
					Source: &ethpb.Checkpoint{
						Epoch: 78,
						Root:  testhelpers.FillByteSlice(32, 79),
					},
					Target: &ethpb.Checkpoint{
						Epoch: 80,
						Root:  testhelpers.FillByteSlice(32, 81),
					},
				},
				Signature:     testhelpers.FillByteSlice(96, 82),
				CommitteeBits: testhelpers.FillByteSlice(8, 83),
			},
			SelectionProof: testhelpers.FillByteSlice(96, 84),
		},
		Signature: testhelpers.FillByteSlice(96, 85),
	}
}
