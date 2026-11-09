package hdiff

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestHezeDiff(t *testing.T) {
	gloasSource, err := gloasState(t, 64)
	require.NoError(t, err)
	hezeSource, err := heze.UpgradeToHeze(gloasSource.Copy())
	require.NoError(t, err)
	require.Equal(t, version.Heze, hezeSource.Version())

	t.Run("round trip keeps inclusion list bits", func(t *testing.T) {
		target := hezeSource.Copy()
		require.NoError(t, target.SetSlot(hezeSource.Slot()+1))
		bid := &ethpb.ExecutionPayloadBidHeze{
			ParentBlockHash:       make([]byte, 32),
			ParentBlockRoot:       make([]byte, 32),
			BlockHash:             make([]byte, 32),
			PrevRandao:            make([]byte, 32),
			FeeRecipient:          make([]byte, 20),
			ExecutionRequestsRoot: make([]byte, 32),
			InclusionListBits:     []byte{0x03, 0x80},
		}
		wrapped, err := blocks.WrappedROExecutionPayloadBidHeze(bid)
		require.NoError(t, err)
		require.NoError(t, target.SetExecutionPayloadBid(wrapped))

		hdiffBytes, err := Diff(hezeSource, target)
		require.NoError(t, err)
		result, err := ApplyDiff(t.Context(), hezeSource.Copy(), hdiffBytes)
		require.NoError(t, err)
		requireEqualState(t, target, result)

		got, err := result.LatestExecutionPayloadBid()
		require.NoError(t, err)
		require.DeepEqual(t, bitfield.Bitvector16{0x03, 0x80}, got.InclusionListBits())
	})

	t.Run("cross fork from Gloas", func(t *testing.T) {
		hdiffBytes, err := Diff(gloasSource, hezeSource)
		require.NoError(t, err)
		result, err := ApplyDiff(t.Context(), gloasSource.Copy(), hdiffBytes)
		require.NoError(t, err)
		require.Equal(t, version.Heze, result.Version())
		requireEqualState(t, hezeSource, result)
	})
}
