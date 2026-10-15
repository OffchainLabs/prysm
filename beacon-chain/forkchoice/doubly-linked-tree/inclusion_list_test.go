package doublylinkedtree

import (
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// setupInclusionListNode returns a forkchoice with a full node at slot 1 and the Heze fork at hezeEpoch.
func setupInclusionListNode(t *testing.T, hezeEpoch primitives.Epoch) (*ForkChoice, [32]byte) {
	f := setupGloas(t, 0, 0)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = hezeEpoch
	params.OverrideBeaconConfig(cfg)

	ctx := t.Context()
	root := indexToHash(1)
	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, root, params.BeaconConfig().ZeroHash, indexToHash(100), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))
	pe, err := prepareGloasForkchoicePayload(root)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))
	return f, root
}

func TestShouldExtendPayload_InclusionList(t *testing.T) {
	t.Run("Heze node is satisfied by default", func(t *testing.T) {
		f, root := setupInclusionListNode(t, 0)
		require.Equal(t, true, f.IsPayloadInclusionListSatisfied(root))
		require.Equal(t, true, f.store.shouldExtendPayload(f.store.fullNodeByRoot[root]))
	})
	t.Run("unsatisfied Heze node is not extended", func(t *testing.T) {
		f, root := setupInclusionListNode(t, 0)
		fn := f.store.fullNodeByRoot[root]
		for i := uint64(0); i <= fieldparams.PTCSize/2; i++ {
			fn.node.payloadAvailabilityVote.SetBitAt(i, true)
			fn.node.payloadDataAvailabilityVote.SetBitAt(i, true)
		}
		require.Equal(t, true, f.store.shouldExtendPayload(fn))
		require.NoError(t, f.SetPayloadInclusionListSatisfied(root, false))
		require.Equal(t, false, f.IsPayloadInclusionListSatisfied(root))
		require.Equal(t, false, f.store.shouldExtendPayload(fn))
		require.NoError(t, f.SetPayloadInclusionListSatisfied(root, true))
		require.Equal(t, true, f.store.shouldExtendPayload(fn))
	})
	t.Run("unsatisfied flag is ignored for a Gloas node", func(t *testing.T) {
		f, root := setupInclusionListNode(t, 1)
		require.NoError(t, f.SetPayloadInclusionListSatisfied(root, false))
		require.Equal(t, true, f.IsPayloadInclusionListSatisfied(root))
		require.Equal(t, true, f.store.shouldExtendPayload(f.store.fullNodeByRoot[root]))
	})
	t.Run("unsatisfied Heze node loses the previous slot tie", func(t *testing.T) {
		f, root := setupInclusionListNode(t, 0)
		slot := params.BeaconConfig().SlotDuration()
		f.SetGenesisTime(time.Now().Add(-2*slot - slot/2))
		n := f.store.emptyNodeByRoot[root].node
		require.Equal(t, primitives.Slot(2), f.store.currentSlot())
		require.Equal(t, true, f.store.choosePayloadContent(n).full)
		require.NoError(t, f.SetPayloadInclusionListSatisfied(root, false))
		require.Equal(t, false, f.store.choosePayloadContent(n).full)
	})
}

func TestSetPayloadInclusionListSatisfied_MissingNode(t *testing.T) {
	f, root := setupInclusionListNode(t, 0)
	t.Run("unknown root", func(t *testing.T) {
		require.ErrorIs(t, f.SetPayloadInclusionListSatisfied(indexToHash(42), false), ErrNilNode)
		require.Equal(t, false, f.IsPayloadInclusionListSatisfied(indexToHash(42)))
	})
	t.Run("empty node only", func(t *testing.T) {
		delete(f.store.fullNodeByRoot, root)
		require.ErrorIs(t, f.SetPayloadInclusionListSatisfied(root, false), ErrNilNode)
		require.Equal(t, false, f.IsPayloadInclusionListSatisfied(root))
	})
}
