package backfill

// NOTE: this test documents the gloas payload-fullness edge in buildColumnBatch
// (gloas-backfill-status.md follow-up 2 in ~/kurtosis). It is intentionally kept out of the
// node-jobs-endpoint PR; attach it to the upstream issue/fix instead.

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// gloasBlockWithBid builds a gloas ROBlock at the given slot whose bid carries one KZG
// commitment plus the given payload hashes, which is everything buildColumnBatch reads.
func gloasBlockWithBid(t *testing.T, slot primitives.Slot, blockHash, parentBlockHash byte) blocks.ROBlock {
	pb := util.NewBeaconBlockGloas()
	pb.Block.Slot = slot
	bid := pb.Block.Body.SignedExecutionPayloadBid.Message
	bid.BlockHash = make([]byte, 32)
	bid.BlockHash[0] = blockHash
	bid.ParentBlockHash = make([]byte, 32)
	bid.ParentBlockHash[0] = parentBlockHash
	bid.BlobKzgCommitments = [][]byte{make([]byte, 48)}
	sb, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)
	rob, err := blocks.NewROBlock(sb)
	require.NoError(t, err)
	return rob
}

// TestBuildColumnBatchGloasFullness documents how gloas payload fullness drives column
// requirements in buildColumnBatch, including the batch-boundary edge: the last block of a
// batch has no in-batch child to testify to its fullness, so it is assumed full and its
// custody columns are required even if its payload was actually withheld. Because
// batch.transitionToNext keeps a batch in the column-fetch state while columnsNeeded() > 0,
// a payload-withheld slot at a batch boundary demands sidecars no peer can serve.
func TestBuildColumnBatchGloasFullness(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if params.BeaconConfig().FuluForkEpoch == params.BeaconConfig().FarFutureEpoch {
		params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().DenebForkEpoch + 4096*2
	}
	fuluSlot, err := slots.EpochStart(params.BeaconConfig().FuluForkEpoch)
	require.NoError(t, err)

	// Three consecutive gloas blocks:
	// a's payload (hash 0xaa) is proven revealed by b's bid building on it.
	// b's payload (hash 0xbb) was withheld: c's bid builds on 0xaa, not 0xbb.
	// c is the last block in the batch, so no child can testify either way.
	a := gloasBlockWithBid(t, fuluSlot, 0xaa, 0x99)
	b := gloasBlockWithBid(t, fuluSlot+1, 0xbb, 0xaa)
	c := gloasBlockWithBid(t, fuluSlot+2, 0xcc, 0xaa)

	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	_, _, err = p.UpdateCustodyInfo(0, 4)
	require.NoError(t, err)
	store := filesystem.NewEphemeralDataColumnStorage(t)
	cb, err := buildColumnBatch(ctx, batch{begin: fuluSlot, end: fuluSlot + 10}, verifiedROBlocks{a, b, c}, p, store, mockCurrentSpecNeeds())
	require.NoError(t, err)
	require.NotNil(t, cb)
	require.Equal(t, 3, len(cb.toDownload))

	custody, err := currentCustodiedColumns(ctx, p)
	require.NoError(t, err)
	require.Equal(t, true, custody.Count() > 0)

	// a: payload revealed, so its custody columns are required.
	require.Equal(t, custody.Count(), cb.toDownload[a.Root()].remaining.Count())
	// b: payload withheld, so no columns exist and none are required.
	require.Equal(t, 0, cb.toDownload[b.Root()].remaining.Count())
	// c: last in batch, fullness unknowable — assumed full, custody columns required.
	// If c's payload was actually withheld, these sidecars do not exist on any peer and
	// the batch cannot leave the column-fetch state until the retention window passes it.
	require.Equal(t, custody.Count(), cb.toDownload[c.Root()].remaining.Count())
}
