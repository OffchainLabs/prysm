package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	testhelpers "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/test-helpers"
	"go.uber.org/mock/gomock"
)

func TestProposeBeaconBlock_Phase0(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	phase0Block := generateSignedPhase0Block()

	genericSignedBlock := &ethpb.GenericSignedBeaconBlock{}
	genericSignedBlock.Block = phase0Block

	jsonPhase0Block := &structs.SignedBeaconBlock{
		Signature: hexutil.Encode(phase0Block.Phase0.Signature),
		Message: &structs.BeaconBlock{
			ParentRoot:    hexutil.Encode(phase0Block.Phase0.Block.ParentRoot),
			ProposerIndex: uint64ToString(phase0Block.Phase0.Block.ProposerIndex),
			Slot:          uint64ToString(phase0Block.Phase0.Block.Slot),
			StateRoot:     hexutil.Encode(phase0Block.Phase0.Block.StateRoot),
			Body: &structs.BeaconBlockBody{
				Attestations:      jsonifyAttestations(phase0Block.Phase0.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(phase0Block.Phase0.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(phase0Block.Phase0.Block.Body.Deposits),
				Eth1Data:          jsonifyEth1Data(phase0Block.Phase0.Block.Body.Eth1Data),
				Graffiti:          hexutil.Encode(phase0Block.Phase0.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(phase0Block.Phase0.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(phase0Block.Phase0.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(phase0Block.Phase0.Block.Body.VoluntaryExits),
			},
		},
	}

	marshalledBlock, err := json.Marshal(jsonPhase0Block)
	require.NoError(t, err)

	ctx := context.Background()

	// Make sure that what we send in the POST body is the marshalled version of the protobuf block
	headers := map[string]string{"Eth-Consensus-Version": "phase0"}
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

	expectedBlockRoot, err := phase0Block.Phase0.Block.HashTreeRoot()
	require.NoError(t, err)

	// Make sure that the block root is set
	assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
}

func generateSignedPhase0Block() *ethpb.GenericSignedBeaconBlock_Phase0 {
	return &ethpb.GenericSignedBeaconBlock_Phase0{
		Phase0: &ethpb.SignedBeaconBlock{
			Block:     testhelpers.GenerateProtoPhase0BeaconBlock(),
			Signature: testhelpers.FillByteSlice(96, 110),
		},
	}
}
