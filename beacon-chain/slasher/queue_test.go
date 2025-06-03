package slasher

import (
	"testing"

	slashertypes "github.com/OffchainLabs/prysm/v6/beacon-chain/slasher/types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func Test_attestationsQueue(t *testing.T) {
	t.Run("push_and_dequeue", func(tt *testing.T) {
		attQueue := newAttestationsQueue()
		wantedAtts := []*slashertypes.IndexedAttestationWrapper{
			createAttestationWrapperEmptySig(t, version.Phase0, 0, 1, []uint64{1}, make([]byte, 32)),
			createAttestationWrapperEmptySig(t, version.Phase0, 1, 2, []uint64{1}, make([]byte, 32)),
		}
		attQueue.push(wantedAtts[0])
		attQueue.push(wantedAtts[1])
		require.DeepEqual(t, 2, attQueue.size())

		received := attQueue.dequeue()
		require.DeepEqual(t, 0, attQueue.size())
		require.DeepEqual(t, wantedAtts, received)
	})

	t.Run("extend_and_dequeue", func(tt *testing.T) {
		attQueue := newAttestationsQueue()
		wantedAtts := []*slashertypes.IndexedAttestationWrapper{
			createAttestationWrapperEmptySig(t, version.Phase0, 0, 1, []uint64{1}, make([]byte, 32)),
			createAttestationWrapperEmptySig(t, version.Phase0, 1, 2, []uint64{1}, make([]byte, 32)),
		}
		attQueue.extend(wantedAtts)
		require.DeepEqual(t, 2, attQueue.size())

		received := attQueue.dequeue()
		require.DeepEqual(t, 0, attQueue.size())
		require.DeepEqual(t, wantedAtts, received)
	})
}

func Test_blocksQueue(t *testing.T) {
	t.Run("push_and_dequeue", func(tt *testing.T) {
		blkQueue := newBlocksQueue()
		wantedBlks := []*slashertypes.SignedBlockHeaderWrapper{
			createProposalWrapper(t, 0, primitives.ValidatorIndex(1), make([]byte, 32)),
			createProposalWrapper(t, 1, primitives.ValidatorIndex(1), make([]byte, 32)),
		}
		blkQueue.push(wantedBlks[0])
		blkQueue.push(wantedBlks[1])
		require.DeepEqual(t, 2, blkQueue.size())

		received := blkQueue.dequeue()
		require.DeepEqual(t, 0, blkQueue.size())
		require.DeepEqual(t, wantedBlks, received)
	})

	t.Run("extend_and_dequeue", func(tt *testing.T) {
		blkQueue := newBlocksQueue()
		wantedBlks := []*slashertypes.SignedBlockHeaderWrapper{
			createProposalWrapper(t, 0, primitives.ValidatorIndex(1), make([]byte, 32)),
			createProposalWrapper(t, 1, primitives.ValidatorIndex(1), make([]byte, 32)),
		}
		blkQueue.extend(wantedBlks)
		require.DeepEqual(t, 2, blkQueue.size())

		received := blkQueue.dequeue()
		require.DeepEqual(t, 0, blkQueue.size())
		require.DeepEqual(t, wantedBlks, received)
	})
}
