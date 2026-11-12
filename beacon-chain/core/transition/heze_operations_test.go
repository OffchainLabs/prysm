package transition_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestVerifyOperationLengths_Heze(t *testing.T) {
	st, err := util.NewBeaconStateHeze()
	require.NoError(t, err)
	require.IsNil(t, st.Eth1Data())

	t.Run("empty body", func(t *testing.T) {
		signed, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockHeze())
		require.NoError(t, err)
		_, err = transition.VerifyOperationLengths(t.Context(), st, signed.Block())
		require.NoError(t, err)
	})

	t.Run("too many voluntary exits", func(t *testing.T) {
		blk := util.NewBeaconBlockHeze()
		n := params.BeaconConfig().MaxVoluntaryExits + 1
		blk.Block.Body.VoluntaryExits = make([]*ethpb.SignedVoluntaryExit, n)
		for i := range blk.Block.Body.VoluntaryExits {
			blk.Block.Body.VoluntaryExits[i] = &ethpb.SignedVoluntaryExit{Exit: &ethpb.VoluntaryExit{}, Signature: make([]byte, 96)}
		}
		signed, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		_, err = transition.VerifyOperationLengths(t.Context(), st, signed.Block())
		require.ErrorContains(t, "number of voluntary exits", err)
	})
}
