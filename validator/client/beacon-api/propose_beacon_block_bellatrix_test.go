package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	testhelpers "github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/test-helpers"
	"go.uber.org/mock/gomock"
)

func TestProposeBeaconBlock_Bellatrix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	bellatrixBlock := generateSignedBellatrixBlock()

	genericSignedBlock := &ethpb.GenericSignedBeaconBlock{}
	genericSignedBlock.Block = bellatrixBlock

	jsonBellatrixBlock, err := structs.SignedBeaconBlockBellatrixFromConsensus(bellatrixBlock.Bellatrix)
	require.NoError(t, err)

	marshalledBlock, err := json.Marshal(jsonBellatrixBlock)
	require.NoError(t, err)

	ctx := context.Background()

	// Make sure that what we send in the POST body is the marshalled version of the protobuf block
	headers := map[string]string{"Eth-Consensus-Version": "bellatrix"}
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/beacon/blocks",
		headers,
		bytes.NewBuffer(marshalledBlock),
		nil,
	)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	proposeResponse, err := validatorClient.proposeBeaconBlock(ctx, genericSignedBlock)
	assert.NoError(t, err)
	require.NotNil(t, proposeResponse)

	expectedBlockRoot, err := bellatrixBlock.Bellatrix.Block.HashTreeRoot()
	require.NoError(t, err)

	// Make sure that the block root is set
	assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
}

func generateSignedBellatrixBlock() *ethpb.GenericSignedBeaconBlock_Bellatrix {
	return &ethpb.GenericSignedBeaconBlock_Bellatrix{
		Bellatrix: &ethpb.SignedBeaconBlockBellatrix{
			Block:     testhelpers.GenerateProtoBellatrixBeaconBlock(),
			Signature: testhelpers.FillByteSlice(96, 127),
		},
	}
}
