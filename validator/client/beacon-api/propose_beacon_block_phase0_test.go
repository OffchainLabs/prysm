package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	testhelpers "github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/test-helpers"
	"go.uber.org/mock/gomock"
)

func TestProposeBeaconBlock_Phase0(t *testing.T) {
	t.Run("SSZ_Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

		phase0Block := generateSignedPhase0Block()

		genericSignedBlock := &ethpb.GenericSignedBeaconBlock{}
		genericSignedBlock.Block = phase0Block

		// Marshal SSZ for comparison
		marshalledSSZ, err := phase0Block.Phase0.MarshalSSZ()
		require.NoError(t, err)

		ctx := t.Context()

		// Expect PostSSZ with SSZ data
		headers := map[string]string{
			"Eth-Consensus-Version": "phase0",
			"Content-Type": "application/octet-stream",
		}
		jsonRestHandler.EXPECT().PostSSZ(
			gomock.Any(),
			"/eth/v2/beacon/blocks",
			headers,
			bytes.NewBuffer(marshalledSSZ),
		).Return(nil, nil, nil)

		validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
		proposeResponse, err := validatorClient.proposeBeaconBlock(ctx, genericSignedBlock)
		assert.NoError(t, err)
		require.NotNil(t, proposeResponse)

		expectedBlockRoot, err := phase0Block.Phase0.Block.HashTreeRoot()
		require.NoError(t, err)

		// Make sure that the block root is set
		assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
	})

	t.Run("SSZ_Fails_Fallback_To_JSON", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

		phase0Block := generateSignedPhase0Block()

		genericSignedBlock := &ethpb.GenericSignedBeaconBlock{}
		genericSignedBlock.Block = phase0Block

		jsonPhase0Block := structs.SignedBeaconBlockPhase0FromConsensus(phase0Block.Phase0)
		marshalledJSON, err := json.Marshal(jsonPhase0Block)
		require.NoError(t, err)

		ctx := t.Context()

		// First expect PostSSZ to fail
		sszHeaders := map[string]string{
			"Eth-Consensus-Version": "phase0",
			"Content-Type": "application/octet-stream",
		}
		jsonRestHandler.EXPECT().PostSSZ(
			gomock.Any(),
			"/eth/v2/beacon/blocks",
			sszHeaders,
			gomock.Any(),
		).Return(nil, nil, errors.New("SSZ not supported"))

		// Then expect fallback to JSON Post
		jsonHeaders := map[string]string{"Eth-Consensus-Version": "phase0"}
		jsonRestHandler.EXPECT().Post(
			gomock.Any(),
			"/eth/v2/beacon/blocks",
			jsonHeaders,
			bytes.NewBuffer(marshalledJSON),
			nil,
		).Return(nil)

		validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
		proposeResponse, err := validatorClient.proposeBeaconBlock(ctx, genericSignedBlock)
		assert.NoError(t, err)
		require.NotNil(t, proposeResponse)

		expectedBlockRoot, err := phase0Block.Phase0.Block.HashTreeRoot()
		require.NoError(t, err)

		// Make sure that the block root is set
		assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
	})
}

func generateSignedPhase0Block() *ethpb.GenericSignedBeaconBlock_Phase0 {
	return &ethpb.GenericSignedBeaconBlock_Phase0{
		Phase0: &ethpb.SignedBeaconBlock{
			Block:     testhelpers.GenerateProtoPhase0BeaconBlock(),
			Signature: testhelpers.FillByteSlice(96, 110),
		},
	}
}
