package validator

import (
	"testing"

	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSetGloasExecutionData(t *testing.T) {
	parentRoot := [32]byte{1, 2, 3}
	slot := primitives.Slot(100)
	proposerIndex := primitives.ValidatorIndex(42)

	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:          slot,
			ProposerIndex: proposerIndex,
			ParentRoot:    parentRoot[:],
			Body:          &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	payload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	}
	ed, err := consensusblocks.WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData:     ed,
		Bid:               primitives.ZeroWei(),
		BlobsBundler:      nil,
		ExecutionRequests: &enginev1.ExecutionRequests{},
	}

	vs := &Server{}

	err = vs.setGloasExecutionData(t.Context(), sBlk, local)
	require.NoError(t, err)

	// Verify the signed bid was set on the block.
	signedBid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	require.NotNil(t, signedBid)
	require.NotNil(t, signedBid.Message)

	// Per spec (process_execution_payload_bid): for self-builds,
	// signature must be G2 point-at-infinity.
	require.DeepEqual(t, common.InfiniteSignature[:], signedBid.Signature)

	// Verify bid fields.
	bid := signedBid.Message
	require.Equal(t, slot, bid.Slot)
	require.Equal(t, primitives.BuilderIndex(proposerIndex), bid.BuilderIndex)
	require.DeepEqual(t, parentRoot[:], bid.ParentBlockRoot)
	require.Equal(t, uint64(0), bid.Value)
	require.Equal(t, uint64(0), bid.ExecutionPayment)
}

func TestSetGloasExecutionData_NilPayload(t *testing.T) {
	sBlk, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       1,
			ParentRoot: make([]byte, 32),
			Body:       &ethpb.BeaconBlockBodyGloas{},
		},
	})
	require.NoError(t, err)

	vs := &Server{}

	err = vs.setGloasExecutionData(t.Context(), sBlk, nil)
	require.ErrorContains(t, "local execution payload is nil", err)

	err = vs.setGloasExecutionData(t.Context(), sBlk, &consensusblocks.GetPayloadResponse{})
	require.ErrorContains(t, "local execution payload is nil", err)
}
