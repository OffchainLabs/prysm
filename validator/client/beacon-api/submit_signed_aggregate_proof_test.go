package beacon_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	testhelpers "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/test-helpers"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

func TestSubmitSignedAggregateSelectionProof_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProof := generateSignedAggregateAndProofJson()
	elem, err := signedAggregateAndProof.MarshalSSZ()
	require.NoError(t, err)
	sszBody := ssz.MarshalVariableList(elem)

	ctx := t.Context()
	headers := map[string]string{"Eth-Consensus-Version": version.String(signedAggregateAndProof.Message.Version())}
	handler := mock.NewMockHandler(ctrl)
	expectPostSSZWithFallback(handler)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(sszBody),
	).Return(
		nil,
	).Times(1)

	attestationDataRoot, err := signedAggregateAndProof.Message.Aggregate.Data.HashTreeRoot()
	require.NoError(t, err)

	validatorClient := &beaconApiValidatorClient{handler: handler}
	resp, err := validatorClient.submitSignedAggregateSelectionProof(ctx, &ethpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: signedAggregateAndProof,
	})
	require.NoError(t, err)
	assert.DeepEqual(t, attestationDataRoot[:], resp.AttestationDataRoot)
}

func TestSubmitSignedAggregateSelectionProof_JsonFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProof := generateSignedAggregateAndProofJson()
	marshalledSignedAggregateSignedAndProof, err := json.Marshal([]*structs.SignedAggregateAttestationAndProof{jsonifySignedAggregateAndProof(signedAggregateAndProof)})
	require.NoError(t, err)

	ctx := t.Context()
	headers := map[string]string{"Eth-Consensus-Version": version.String(signedAggregateAndProof.Message.Version())}
	handler := mock.NewMockHandler(ctrl)
	expectPostSSZWithFallback(handler)
	handler.EXPECT().PostSSZ(gomock.Any(), "/eth/v2/validator/aggregate_and_proofs", headers, gomock.Any()).Return(
		&httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType, Message: "unsupported media type"},
	).Times(1)
	handler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProof),
		nil,
	).Return(
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{handler: handler}
	_, err = validatorClient.submitSignedAggregateSelectionProof(ctx, &ethpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: signedAggregateAndProof,
	})
	require.NoError(t, err)
}

func TestSubmitSignedAggregateSelectionProof_BadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProof := generateSignedAggregateAndProofJson()

	ctx := t.Context()
	headers := map[string]string{"Eth-Consensus-Version": version.String(signedAggregateAndProof.Message.Version())}
	handler := mock.NewMockHandler(ctrl)
	expectPostSSZWithFallback(handler)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		gomock.Any(),
	).Return(
		errors.New("bad request"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{handler: handler}
	_, err := validatorClient.submitSignedAggregateSelectionProof(ctx, &ethpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: signedAggregateAndProof,
	})
	assert.ErrorContains(t, "bad request", err)
}

func TestSubmitSignedAggregateSelectionProofElectra_Valid(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().ElectraForkEpoch = 0
	params.BeaconConfig().FuluForkEpoch = 100

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProofElectra := generateSignedAggregateAndProofElectraJson()
	elem, err := signedAggregateAndProofElectra.MarshalSSZ()
	require.NoError(t, err)
	sszBody := ssz.MarshalVariableList(elem)

	ctx := t.Context()
	expectedVersion := version.String(slots.ToForkVersion(signedAggregateAndProofElectra.Message.Aggregate.Data.Slot))
	headers := map[string]string{"Eth-Consensus-Version": expectedVersion}
	handler := mock.NewMockHandler(ctrl)
	expectPostSSZWithFallback(handler)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(sszBody),
	).Return(
		nil,
	).Times(1)

	attestationDataRoot, err := signedAggregateAndProofElectra.Message.Aggregate.Data.HashTreeRoot()
	require.NoError(t, err)

	validatorClient := &beaconApiValidatorClient{handler: handler}
	resp, err := validatorClient.submitSignedAggregateSelectionProofElectra(ctx, &ethpb.SignedAggregateSubmitElectraRequest{
		SignedAggregateAndProof: signedAggregateAndProofElectra,
	})
	require.NoError(t, err)
	assert.DeepEqual(t, attestationDataRoot[:], resp.AttestationDataRoot)
}

func TestSubmitSignedAggregateSelectionProofElectra_JsonFallback(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().ElectraForkEpoch = 0
	params.BeaconConfig().FuluForkEpoch = 100

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProofElectra := generateSignedAggregateAndProofElectraJson()
	marshalledSignedAggregateSignedAndProofElectra, err := json.Marshal([]*structs.SignedAggregateAttestationAndProofElectra{jsonifySignedAggregateAndProofElectra(signedAggregateAndProofElectra)})
	require.NoError(t, err)

	ctx := t.Context()
	expectedVersion := version.String(slots.ToForkVersion(signedAggregateAndProofElectra.Message.Aggregate.Data.Slot))
	headers := map[string]string{"Eth-Consensus-Version": expectedVersion}
	handler := mock.NewMockHandler(ctrl)
	expectPostSSZWithFallback(handler)
	handler.EXPECT().PostSSZ(gomock.Any(), "/eth/v2/validator/aggregate_and_proofs", headers, gomock.Any()).Return(
		&httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType, Message: "unsupported media type"},
	).Times(1)
	handler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(marshalledSignedAggregateSignedAndProofElectra),
		nil,
	).Return(
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{handler: handler}
	_, err = validatorClient.submitSignedAggregateSelectionProofElectra(ctx, &ethpb.SignedAggregateSubmitElectraRequest{
		SignedAggregateAndProof: signedAggregateAndProofElectra,
	})
	require.NoError(t, err)
}

func TestSubmitSignedAggregateSelectionProofElectra_BadRequest(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().ElectraForkEpoch = 0
	params.BeaconConfig().FuluForkEpoch = 100

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProofElectra := generateSignedAggregateAndProofElectraJson()

	ctx := t.Context()
	expectedVersion := version.String(slots.ToForkVersion(signedAggregateAndProofElectra.Message.Aggregate.Data.Slot))
	headers := map[string]string{"Eth-Consensus-Version": expectedVersion}
	handler := mock.NewMockHandler(ctrl)
	expectPostSSZWithFallback(handler)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		gomock.Any(),
	).Return(
		errors.New("bad request"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{handler: handler}
	_, err := validatorClient.submitSignedAggregateSelectionProofElectra(ctx, &ethpb.SignedAggregateSubmitElectraRequest{
		SignedAggregateAndProof: signedAggregateAndProofElectra,
	})
	assert.ErrorContains(t, "bad request", err)
}

func TestSubmitSignedAggregateSelectionProofElectra_FuluVersion(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().ElectraForkEpoch = 0
	params.BeaconConfig().FuluForkEpoch = 1

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signedAggregateAndProofElectra := generateSignedAggregateAndProofElectraJson()
	elem, err := signedAggregateAndProofElectra.MarshalSSZ()
	require.NoError(t, err)
	sszBody := ssz.MarshalVariableList(elem)

	ctx := t.Context()
	expectedVersion := version.String(slots.ToForkVersion(signedAggregateAndProofElectra.Message.Aggregate.Data.Slot))
	headers := map[string]string{"Eth-Consensus-Version": expectedVersion}
	handler := mock.NewMockHandler(ctrl)
	expectPostSSZWithFallback(handler)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v2/validator/aggregate_and_proofs",
		headers,
		bytes.NewBuffer(sszBody),
	).Return(
		nil,
	).Times(1)

	attestationDataRoot, err := signedAggregateAndProofElectra.Message.Aggregate.Data.HashTreeRoot()
	require.NoError(t, err)

	validatorClient := &beaconApiValidatorClient{handler: handler}
	resp, err := validatorClient.submitSignedAggregateSelectionProofElectra(ctx, &ethpb.SignedAggregateSubmitElectraRequest{
		SignedAggregateAndProof: signedAggregateAndProofElectra,
	})
	require.NoError(t, err)
	assert.DeepEqual(t, attestationDataRoot[:], resp.AttestationDataRoot)
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
