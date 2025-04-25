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

func TestProposeBeaconBlock_BlindedElectra(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	var block structs.SignedBlindedBeaconBlockElectra
	err := json.Unmarshal([]byte(rpctesting.BlindedElectraBlock), &block)
	require.NoError(t, err)
	genericSignedBlock, err := block.ToGeneric()
	require.NoError(t, err)

	electraBytes, err := json.Marshal(block)
	require.NoError(t, err)
	// Make sure that what we send in the POST body is the marshalled version of the protobuf block
	headers := map[string]string{"Eth-Consensus-Version": "electra"}
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/beacon/blinded_blocks",
		headers,
		bytes.NewBuffer(electraBytes),
		nil,
	)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	proposeResponse, err := validatorClient.proposeBeaconBlock(context.Background(), genericSignedBlock)
	assert.NoError(t, err)
	require.NotNil(t, proposeResponse)

	expectedBlockRoot, err := genericSignedBlock.GetBlindedElectra().HashTreeRoot()
	require.NoError(t, err)

	// Make sure that the block root is set
	assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
}
