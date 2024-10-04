package das

import (
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func TestFullCommitmentsToCheck(t *testing.T) {
	windowSlots, err := slots.EpochEnd(params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)
	require.NoError(t, err)
	commits := [][]byte{
		bytesutil.PadTo([]byte("a"), 48),
		bytesutil.PadTo([]byte("b"), 48),
		bytesutil.PadTo([]byte("c"), 48),
		bytesutil.PadTo([]byte("d"), 48),
	}
	cases := []struct {
		name    string
		commits [][]byte
		block   func(*testing.T) blocks.ROBlock
		slot    primitives.Slot
		err     error
	}{
		{
			name: "pre deneb",
			block: func(t *testing.T) blocks.ROBlock {
				bb := util.NewBeaconBlockBellatrix()
				sb, err := blocks.NewSignedBeaconBlock(bb)
				require.NoError(t, err)
				rb, err := blocks.NewROBlock(sb)
				require.NoError(t, err)
				return rb
			},
		},
		{
			name: "commitments within da",
			block: func(t *testing.T) blocks.ROBlock {
				d := util.NewBeaconBlockDeneb()
				d.Block.Body.BlobKzgCommitments = commits
				d.Block.Slot = 100
				sb, err := blocks.NewSignedBeaconBlock(d)
				require.NoError(t, err)
				rb, err := blocks.NewROBlock(sb)
				require.NoError(t, err)
				return rb
			},
			commits: commits,
			slot:    100,
		},
		{
			name: "commitments outside da",
			block: func(t *testing.T) blocks.ROBlock {
				d := util.NewBeaconBlockDeneb()
				// block is from slot 0, "current slot" is window size +1 (so outside the window)
				d.Block.Body.BlobKzgCommitments = commits
				sb, err := blocks.NewSignedBeaconBlock(d)
				require.NoError(t, err)
				rb, err := blocks.NewROBlock(sb)
				require.NoError(t, err)
				return rb
			},
			slot: windowSlots + 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resetFlags := flags.Get()
			gFlags := new(flags.GlobalFlags)
			gFlags.SubscribeToAllSubnets = true
			flags.Init(gFlags)
			defer flags.Init(resetFlags)

			b := c.block(t)
			co, err := fullCommitmentsToCheck(enode.ID{}, b, c.slot)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
			} else {
				require.NoError(t, err)
			}
			for i := 0; i < len(co); i++ {
				require.DeepEqual(t, c.commits, co[i])
			}
		})
	}
}
