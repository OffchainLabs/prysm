package structs

import (
	"testing"

	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func testEnvelopeProto() *eth.ExecutionPayloadEnvelope {
	return &eth.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    fillByteSlice(common.HashLength, 0xaa),
			FeeRecipient:  fillByteSlice(20, 0xbb),
			StateRoot:     fillByteSlice(32, 0xcc),
			ReceiptsRoot:  fillByteSlice(32, 0xdd),
			LogsBloom:     fillByteSlice(256, 0xee),
			PrevRandao:    fillByteSlice(32, 0xff),
			BaseFeePerGas: fillByteSlice(32, 0x11),
			BlockHash:     fillByteSlice(common.HashLength, 0x22),
		},
		ExecutionRequests: &enginev1.ExecutionRequests{},
		BuilderIndex:      7,
		BeaconBlockRoot:   fillByteSlice(32, 0x33),
		Slot:              42,
		StateRoot:         fillByteSlice(32, 0x44),
	}
}

func TestExecutionPayloadEnvelopeFromConsensus(t *testing.T) {
	env := testEnvelopeProto()
	result, err := ExecutionPayloadEnvelopeFromConsensus(env)
	require.NoError(t, err)
	require.NotNil(t, result.Payload)
	require.Equal(t, hexutil.Encode(env.Payload.ParentHash), result.Payload.ParentHash)
	require.Equal(t, "7", result.BuilderIndex)
	require.Equal(t, hexutil.Encode(env.BeaconBlockRoot), result.BeaconBlockRoot)
	require.Equal(t, "42", result.Slot)
	require.Equal(t, hexutil.Encode(env.StateRoot), result.StateRoot)
	require.NotNil(t, result.ExecutionRequests)
}

func TestExecutionPayloadEnvelopeFromConsensus_NilRequests(t *testing.T) {
	env := testEnvelopeProto()
	env.ExecutionRequests = nil
	result, err := ExecutionPayloadEnvelopeFromConsensus(env)
	require.NoError(t, err)
	require.Equal(t, (*ExecutionRequests)(nil), result.ExecutionRequests)
}

func TestBlockContentsGloasFromConsensus(t *testing.T) {
	block := util.NewBeaconBlockGloas().Block
	env := testEnvelopeProto()

	result, err := BlockContentsGloasFromConsensus(block, env)
	require.NoError(t, err)
	require.NotNil(t, result.Block)
	require.NotNil(t, result.Block.Body)
	require.NotNil(t, result.ExecutionPayloadEnvelope)
	require.Equal(t, hexutil.Encode(env.BeaconBlockRoot), result.ExecutionPayloadEnvelope.BeaconBlockRoot)
	require.Equal(t, 0, len(result.KzgProofs))
	require.Equal(t, 0, len(result.Blobs))
}
