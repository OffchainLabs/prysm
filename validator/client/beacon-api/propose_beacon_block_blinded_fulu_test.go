package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	rpctesting "github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared/testing"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
)

func TestProposeBeaconBlock_BlindedFulu(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	var block structs.SignedBlindedBeaconBlockFulu
	err := json.Unmarshal([]byte(rpctesting.BlindedFuluBlock), &block)
	require.NoError(t, err)
	genericSignedBlock, err := block.ToGeneric()
	require.NoError(t, err)

	fuluBytes, err := json.Marshal(block)
	require.NoError(t, err)
	// Make sure that what we send in the POST body is the marshalled version of the protobuf block
	headers := map[string]string{"Eth-Consensus-Version": "fulu"}
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/beacon/blinded_blocks",
		headers,
		bytes.NewBuffer(fuluBytes),
		nil,
	)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	proposeResponse, err := validatorClient.proposeBeaconBlock(context.Background(), genericSignedBlock)
	assert.NoError(t, err)
	require.NotNil(t, proposeResponse)

	expectedBlockRoot, err := genericSignedBlock.GetBlindedFulu().HashTreeRoot()
	require.NoError(t, err)

	// Make sure that the block root is set
	assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
}
