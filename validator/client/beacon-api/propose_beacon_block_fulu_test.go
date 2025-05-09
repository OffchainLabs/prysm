package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	rpctesting "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared/testing"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
)

func TestProposeBeaconBlock_Fulu(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	var blockContents structs.SignedBeaconBlockContentsFulu
	err := json.Unmarshal([]byte(rpctesting.FuluBlockContents), &blockContents)
	require.NoError(t, err)
	genericSignedBlock, err := blockContents.ToGeneric()
	require.NoError(t, err)

	fuluBytes, err := json.Marshal(blockContents)
	require.NoError(t, err)
	// Make sure that what we send in the POST body is the marshalled version of the protobuf block
	headers := map[string]string{"Eth-Consensus-Version": "fulu"}
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/beacon/blocks",
		headers,
		bytes.NewBuffer(fuluBytes),
		nil,
	)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	proposeResponse, err := validatorClient.proposeBeaconBlock(context.Background(), genericSignedBlock)
	assert.NoError(t, err)
	require.NotNil(t, proposeResponse)

	expectedBlockRoot, err := genericSignedBlock.GetFulu().Block.HashTreeRoot()
	require.NoError(t, err)

	// Make sure that the block root is set
	assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
}
