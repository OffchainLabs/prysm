package initialsync

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	prysmsync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
)

// makeGloasBlock creates a Gloas ROBlock with the given slot, parentRoot, and parentBlockHash in the bid.
func makeGloasBlock(t *testing.T, slot primitives.Slot, parentRoot [32]byte, parentBlockHash [32]byte) blocks.ROBlock {
	return makeGloasBlockWithPayload(t, slot, parentRoot, parentBlockHash, [32]byte{})
}

func makeGloasBlockWithPayload(t *testing.T, slot primitives.Slot, parentRoot, parentBlockHash, blockHash [32]byte) blocks.ROBlock {
	blk := util.NewBeaconBlockGloas()
	blk.Block.Slot = slot
	blk.Block.ParentRoot = parentRoot[:]
	blk.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockHash = parentBlockHash[:]
	blk.Block.Body.SignedExecutionPayloadBid.Message.BlockHash = blockHash[:]
	signed, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	ro, err := blocks.NewROBlock(signed)
	require.NoError(t, err)
	return ro
}

// makeEnvelope creates an ROSignedExecutionPayloadEnvelope with the given slot, blockHash, and parentHash.
func makeEnvelope(t *testing.T, slot primitives.Slot, blockHash [32]byte, parentHash [32]byte) interfaces.ROSignedExecutionPayloadEnvelope {
	return makeEnvelopeForRoot(t, slot, [32]byte{}, blockHash, parentHash)
}

func TestCheckAllBlocksBuildOnEmpty(t *testing.T) {
	parentHash := [32]byte{1}
	// Block 0: root will be computed, parentBlockHash = parentHash
	b0 := makeGloasBlock(t, 10, [32]byte{}, parentHash)
	// Block 1: parentRoot = b0.Root(), same parentBlockHash (builds on empty)
	b1 := makeGloasBlock(t, 11, b0.Root(), parentHash)
	// Block 2: parentRoot = b1.Root(), same parentBlockHash (builds on empty)
	b2 := makeGloasBlock(t, 12, b1.Root(), parentHash)

	t.Run("all build on empty", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: b0},
			{Block: b1},
			{Block: b2},
		}
		err := checkAllBlocksBuildOnEmpty(bwb)
		require.NoError(t, err)
	})

	t.Run("block does not descend from previous", func(t *testing.T) {
		// b2's parentRoot is b1.Root(), not b0.Root(), so [b0, b2] is invalid
		bwb := []blocks.BlockWithROSidecars{
			{Block: b0},
			{Block: b2},
		}
		err := checkAllBlocksBuildOnEmpty(bwb)
		require.ErrorContains(t, "does not descend from", err)
	})

	t.Run("different parent block hash", func(t *testing.T) {
		differentHash := [32]byte{2}
		bDiff := makeGloasBlock(t, 11, b0.Root(), differentHash)
		bwb := []blocks.BlockWithROSidecars{
			{Block: b0},
			{Block: bDiff},
		}
		err := checkAllBlocksBuildOnEmpty(bwb)
		require.ErrorContains(t, "does not build on top of the empty block", err)
	})

}

func TestFindFirstForkIndex_Gloas(t *testing.T) {
	fulu := util.NewBeaconBlockFulu()
	signedFulu, err := blocks.NewSignedBeaconBlock(fulu)
	require.NoError(t, err)
	roFulu, err := blocks.NewROBlock(signedFulu)
	require.NoError(t, err)

	gloas := util.NewBeaconBlockGloas()
	signedGloas, err := blocks.NewSignedBeaconBlock(gloas)
	require.NoError(t, err)
	roGloas, err := blocks.NewROBlock(signedGloas)
	require.NoError(t, err)

	deneb := util.NewBeaconBlockDeneb()
	signedDeneb, err := blocks.NewSignedBeaconBlock(deneb)
	require.NoError(t, err)
	roDeneb, err := blocks.NewROBlock(signedDeneb)
	require.NoError(t, err)

	t.Run("all pre-Gloas", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: roDeneb},
			{Block: roFulu},
		}
		idx, err := findFirstForkIndex(bwb, version.Gloas)
		require.NoError(t, err)
		require.Equal(t, 2, idx)
	})

	t.Run("all Gloas", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: roGloas},
			{Block: roGloas},
		}
		idx, err := findFirstForkIndex(bwb, version.Gloas)
		require.NoError(t, err)
		require.Equal(t, 0, idx)
	})

	t.Run("mixed correctly sorted", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: roDeneb},
			{Block: roFulu},
			{Block: roGloas},
		}
		idx, err := findFirstForkIndex(bwb, version.Gloas)
		require.NoError(t, err)
		require.Equal(t, 2, idx)
	})

	t.Run("mixed incorrectly sorted", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: roGloas},
			{Block: roFulu},
		}
		_, err := findFirstForkIndex(bwb, version.Gloas)
		require.NotNil(t, err)
	})
}

func TestValidatePayloadBlockConsistency(t *testing.T) {
	// Setup: create a chain of 3 Gloas blocks where each has a different parent hash
	// (meaning each requires an envelope) and envelopes that match.
	hash0 := [32]byte{0x10}
	hash1 := [32]byte{0x20}
	hash2 := [32]byte{0x30}

	// Block 0: parentBlockHash = hash0
	b0 := makeGloasBlock(t, 10, [32]byte{}, hash0)
	// Block 1: parentRoot = b0.Root(), parentBlockHash = hash1 (different from hash0 => needs envelope)
	b1 := makeGloasBlock(t, 11, b0.Root(), hash1)
	// Block 2: parentRoot = b1.Root(), parentBlockHash = hash2 (different from hash1 => needs envelope)
	b2 := makeGloasBlock(t, 12, b1.Root(), hash2)

	// Envelopes: env0 has blockHash=hash1 (matches b1's parentBlockHash)
	// env1 has blockHash=hash2 (matches b2's parentBlockHash)
	env0 := makeEnvelope(t, 10, hash0, [32]byte{})
	env1 := makeEnvelope(t, 11, hash1, hash0)

	t.Run("consistent envelopes and blocks, envelope is first", func(t *testing.T) {
		f := &blocksFetcher{}
		r := &fetchRequestResponse{
			bwb: []blocks.BlockWithROSidecars{
				{Block: b0},
				{Block: b1},
				{Block: b2},
			},
			envelopes: []interfaces.ROSignedExecutionPayloadEnvelope{env0, env1},
		}
		f.validatePayloadBlockConsistency(r)
		require.NoError(t, r.err)
		require.Equal(t, 2, len(r.envelopes))
	})

	t.Run("not enough envelopes truncates blocks", func(t *testing.T) {
		f := &blocksFetcher{}
		r := &fetchRequestResponse{
			bwb: []blocks.BlockWithROSidecars{
				{Block: b0},
				{Block: b1},
				{Block: b2},
			},
			// Only one envelope, but two are needed
			envelopes: []interfaces.ROSignedExecutionPayloadEnvelope{env0},
		}
		f.validatePayloadBlockConsistency(r)
		// Should truncate bwb to the point where envelopes run out
		require.NoError(t, r.err)
	})

	t.Run("extra envelopes truncated", func(t *testing.T) {
		env2 := makeEnvelope(t, 12, hash2, hash1)
		f := &blocksFetcher{}
		// All blocks have the same parentBlockHash => no envelope transitions needed
		sameHash := [32]byte{0x99}
		sb0 := makeGloasBlock(t, 10, [32]byte{}, sameHash)
		sb1 := makeGloasBlock(t, 11, sb0.Root(), sameHash)

		envFirst := makeEnvelope(t, 10, sameHash, [32]byte{})
		r := &fetchRequestResponse{
			bwb: []blocks.BlockWithROSidecars{
				{Block: sb0},
				{Block: sb1},
			},
			envelopes: []interfaces.ROSignedExecutionPayloadEnvelope{envFirst, env2},
		}
		f.validatePayloadBlockConsistency(r)
		require.NoError(t, r.err)
		// Extra envelope should be truncated
		require.Equal(t, 1, len(r.envelopes))
	})

	t.Run("mismatched envelope from different peer does not wrap ErrInvalidFetchedData", func(t *testing.T) {
		wrongEnv := makeEnvelope(t, 10, [32]byte{0xff}, [32]byte{})
		f := &blocksFetcher{}
		r := &fetchRequestResponse{
			blocksFrom:   "peer1",
			payloadsFrom: "peer2",
			bwb: []blocks.BlockWithROSidecars{
				{Block: b0},
				{Block: b1},
			},
			envelopes: []interfaces.ROSignedExecutionPayloadEnvelope{wrongEnv},
		}
		f.validatePayloadBlockConsistency(r)
		require.ErrorContains(t, "envelope does not match block", r.err)
		require.Equal(t, false, errors.Is(r.err, prysmsync.ErrInvalidFetchedData))
	})

}

func newPayloadTestFetcher(t *testing.T, headSlot primitives.Slot) (*blocksFetcher, *p2ptest.TestP2P) {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()
	ctxMap, err := prysmsync.ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)
	p := p2ptest.NewTestP2P(t)
	store := dbtest.SetupDB(t)
	f := newBlocksFetcher(t.Context(), &blocksFetcherConfig{
		p2p: p, db: store, ctxMap: ctxMap,
		chain: &mock.ChainService{MockHeadSlot: &headSlot, DB: store, FinalizedCheckPoint: &ethpb.Checkpoint{}},
		clock: startup.NewClock(time.Now(), [32]byte{}),
	})
	t.Cleanup(func() { f.cancel(); f.rateLimiter.Free() })
	return f, p
}

func TestValidatePayloadsForImport_Truncation(t *testing.T) {
	old := makeGloasBlock(t, 8, [32]byte{}, [32]byte{1})
	anchor := makeGloasBlock(t, 10, old.Root(), [32]byte{2})
	child := makeGloasBlock(t, 14, anchor.Root(), [32]byte{3})
	next := makeGloasBlock(t, 15, child.Root(), [32]byte{4})
	r := &fetchRequestResponse{
		bwb: []blocks.BlockWithROSidecars{{Block: old}, {Block: anchor}, {Block: child}, {Block: next}},
		envelopes: []interfaces.ROSignedExecutionPayloadEnvelope{
			makeEnvelopeForRoot(t, 10, anchor.Root(), [32]byte{3}, [32]byte{2}),
		},
	}
	f := &blocksFetcher{}
	f.validatePayloadsForImport(r, 1)
	require.NoError(t, r.err)
	require.Equal(t, 3, len(r.bwb))
	require.Equal(t, child.Root(), r.bwb[2].Block.Root())
	require.Equal(t, 1, len(r.envelopes))
}

func TestFetchPayloads_RequiredParent(t *testing.T) {
	parentHash, blockHash := [32]byte{1}, [32]byte{2}
	parent := makeGloasBlockWithPayload(t, 10, [32]byte{}, parentHash, blockHash)
	child := makeGloasBlockWithPayload(t, 14, parent.Root(), blockHash, [32]byte{4})
	emptyChild := makeGloasBlock(t, 14, parent.Root(), parentHash)
	older := makeGloasBlockWithPayload(t, 8, [32]byte{}, [32]byte{3}, parentHash)
	envelope := makeEnvelopeForRoot(t, 10, parent.Root(), blockHash, parentHash)
	childEnvelope := makeEnvelopeForRoot(t, 14, child.Root(), [32]byte{4}, blockHash)
	genesis := makeGloasBlockWithPayload(t, 0, [32]byte{}, parentHash, blockHash)
	genesisChild := makeGloasBlock(t, 1, genesis.Root(), blockHash)
	tests := []struct {
		name           string
		blocks         []blocks.BlockWithROSidecars
		head           primitives.Slot
		rangePayload   interfaces.ROSignedExecutionPayloadEnvelope
		missingParent  bool
		parentFullNode bool
		unknownFork    bool
		rootRequests   int32
		wantPayloads   int
		wantErr        string
	}{
		{name: "genesis full child needs no envelope", blocks: []blocks.BlockWithROSidecars{{Block: genesis}, {Block: genesisChild}}, head: 0},
		{name: "parent envelope missing from range is left for import", blocks: []blocks.BlockWithROSidecars{{Block: child}}, head: 10},
		{name: "parent with imported payload needs no root request", blocks: []blocks.BlockWithROSidecars{{Block: child}}, head: 10, parentFullNode: true, missingParent: true},
		{name: "parent with imported payload in batch needs no root request", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10, parentFullNode: true, missingParent: true},
		{name: "parent with imported payload preserves child payload", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10,
			parentFullNode: true, missingParent: true, rangePayload: childEnvelope, wantPayloads: 1},
		{name: "parent with imported payload skips imported ancestors", blocks: []blocks.BlockWithROSidecars{{Block: older}, {Block: parent}, {Block: child}}, head: 10,
			parentFullNode: true, missingParent: true, rangePayload: makeEnvelopeForRoot(t, 8, older.Root(), parentHash, [32]byte{3})},
		{name: "empty withheld origin", blocks: []blocks.BlockWithROSidecars{{Block: emptyChild}}, head: 10},
		{name: "parent already returned by range", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10, rangePayload: envelope, wantPayloads: 1},
		{name: "wrong range parent hash rejects batch", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10,
			rangePayload: makeEnvelopeForRoot(t, 10, parent.Root(), [32]byte{99}, parentHash), wantPayloads: 1, wantErr: "parent payload envelope does not match block"},
		{name: "wrong range parent slot rejects batch", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10,
			rangePayload: makeEnvelopeForRoot(t, 9, parent.Root(), blockHash, parentHash), wantPayloads: 1, wantErr: "parent payload envelope does not match block"},
		{name: "parent with imported payload still rejects wrong range hash", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10,
			parentFullNode: true, rangePayload: makeEnvelopeForRoot(t, 10, parent.Root(), [32]byte{99}, parentHash), wantPayloads: 1, wantErr: "parent payload envelope does not match block"},
		{name: "all known needs no parent", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 14},
		{name: "all known retains new head payload", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 14, rangePayload: childEnvelope, wantPayloads: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, client := newPayloadTestFetcher(t, tt.head)
			require.NoError(t, f.db.(db.Database).SaveBlock(t.Context(), parent.ReadOnlySignedBeaconBlock))
			if tt.parentFullNode {
				require.NoError(t, f.db.(db.Database).SaveExecutionPayloadEnvelope(t.Context(), envelope.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
				f.chain.(*mock.ChainService).ForkchoiceRoots = map[[32]byte]bool{parent.Root(): true}
			}
			if !tt.unknownFork {
				for _, block := range tt.blocks {
					if block.Block.Block().Slot() <= tt.head {
						require.NoError(t, f.db.(db.Database).SaveBlock(t.Context(), block.Block.ReadOnlySignedBeaconBlock))
					}
				}
			}
			server := p2ptest.NewTestP2P(t)
			client.Connect(server)
			var rootRequests atomic.Int32
			server.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1), func(stream network.Stream) {
				defer func() { assert.NoError(t, stream.Close()) }()
				req := new(ethpb.ExecutionPayloadEnvelopesByRangeRequest)
				assert.NoError(t, server.Encoding().DecodeWithMaxLength(stream, req))
				if tt.rangePayload != nil {
					assert.NoError(t, prysmsync.WriteExecutionPayloadEnvelopeChunk(stream, server.Encoding(), tt.rangePayload.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
				}
				assert.NoError(t, stream.CloseWrite())
			})
			server.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1), func(stream network.Stream) {
				defer func() { assert.NoError(t, stream.Close()) }()
				req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
				assert.NoError(t, server.Encoding().DecodeWithMaxLength(stream, req))
				assert.DeepEqual(t, p2ptypes.ExecutionPayloadEnvelopesByRootReq{parent.Root()}, *req)
				rootRequests.Add(1)
				if !tt.missingParent {
					assert.NoError(t, prysmsync.WriteExecutionPayloadEnvelopeChunk(stream, server.Encoding(), envelope.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
				}
				assert.NoError(t, stream.CloseWrite())
			})
			r := &fetchRequestResponse{bwb: tt.blocks, blocksFrom: server.PeerID(), start: tt.blocks[0].Block.Block().Slot(), count: 8}
			f.fetchPayloads(t.Context(), r, nil)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, r.err)
				require.Equal(t, true, errors.Is(r.err, prysmsync.ErrInvalidFetchedData))
			} else if tt.missingParent && !tt.parentFullNode {
				require.ErrorContains(t, "missing payload envelope for FULL parent", r.err)
			} else {
				require.NoError(t, r.err)
			}
			if tt.parentFullNode && tt.wantErr == "" {
				require.Equal(t, server.PeerID(), r.payloadsFrom)
				downscores, err := client.Peers().Scorers().BadResponsesScorer().Count(server.PeerID())
				require.NoError(t, err)
				require.Equal(t, 0, downscores)
			}
			require.Equal(t, tt.rootRequests, rootRequests.Load())
			require.Equal(t, tt.wantPayloads, len(r.envelopes))
			require.Equal(t, len(tt.blocks), len(r.bwb))
			if tt.rootRequests > 0 && !tt.missingParent {
				first, err := r.envelopes[0].Envelope()
				require.NoError(t, err)
				require.Equal(t, parent.Root(), first.BeaconBlockRoot())
			}
		})
	}
}

func TestFetchPayloads_RangeCountLimit(t *testing.T) {
	for _, test := range []struct {
		name             string
		count            uint64
		limit            uint64
		firstSlot        primitives.Slot
		wantStart        primitives.Slot
		wantCount        uint64
		wantRootRequests int32
		parentUnknown    bool
	}{
		{name: "default batch", count: 64, limit: 128, firstSlot: 101, wantStart: 100, wantCount: 65},
		{name: "extra slot fits", count: 127, limit: 128, firstSlot: 101, wantStart: 100, wantCount: 128},
		{name: "maximum batch", count: 128, limit: 128, firstSlot: 101, wantStart: 101, wantCount: 128},
		{name: "maximum batch with leading empty slots", count: 128, limit: 128, firstSlot: 105, wantStart: 105, wantCount: 124},
		{name: "configured payload limit", count: 4, limit: 4, firstSlot: 101, wantStart: 101, wantCount: 4},
		{name: "prefetched parent not yet known", count: 128, limit: 128, firstSlot: 101, wantStart: 101, wantCount: 128, parentUnknown: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			f, client := newPayloadTestFetcher(t, 100)
			params.BeaconConfig().MaxRequestPayloads = test.limit
			ancestorHash, originHash, firstHash, lastHash := [32]byte{1}, [32]byte{2}, [32]byte{3}, [32]byte{4}
			origin := makeGloasBlockWithPayload(t, 100, [32]byte{}, ancestorHash, originHash)
			first := makeGloasBlockWithPayload(t, test.firstSlot, origin.Root(), originHash, firstHash)
			lastSlot := primitives.Slot(100 + test.count)
			last := makeGloasBlockWithPayload(t, lastSlot, first.Root(), firstHash, lastHash)
			if !test.parentUnknown {
				require.NoError(t, f.db.(db.Database).SaveBlock(ctx, origin.ReadOnlySignedBeaconBlock))
			}
			available := []interfaces.ROSignedExecutionPayloadEnvelope{
				makeEnvelopeForRoot(t, 100, origin.Root(), originHash, ancestorHash),
				makeEnvelopeForRoot(t, test.firstSlot, first.Root(), firstHash, originHash),
				makeEnvelopeForRoot(t, lastSlot, last.Root(), lastHash, firstHash),
			}
			server := p2ptest.NewTestP2P(t)
			client.Connect(server)
			var rangeRequests, rootRequests atomic.Int32
			server.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1), func(stream network.Stream) {
				defer func() { assert.NoError(t, stream.Close()) }()
				req := new(ethpb.ExecutionPayloadEnvelopesByRangeRequest)
				assert.NoError(t, server.Encoding().DecodeWithMaxLength(stream, req))
				rangeRequests.Add(1)
				assert.Equal(t, test.wantStart, req.StartSlot)
				assert.Equal(t, test.wantCount, req.Count)
				for _, envelope := range available {
					message, err := envelope.Envelope()
					assert.NoError(t, err)
					if message.Slot() >= req.StartSlot && message.Slot() < req.StartSlot.Add(req.Count) {
						assert.NoError(t, prysmsync.WriteExecutionPayloadEnvelopeChunk(stream, server.Encoding(), envelope.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
					}
				}
				assert.NoError(t, stream.CloseWrite())
			})
			server.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1), func(stream network.Stream) {
				defer func() { assert.NoError(t, stream.Close()) }()
				req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
				assert.NoError(t, server.Encoding().DecodeWithMaxLength(stream, req))
				assert.DeepEqual(t, p2ptypes.ExecutionPayloadEnvelopesByRootReq{origin.Root()}, *req)
				rootRequests.Add(1)
				assert.NoError(t, prysmsync.WriteExecutionPayloadEnvelopeChunk(stream, server.Encoding(), available[0].Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
				assert.NoError(t, stream.CloseWrite())
			})
			r := &fetchRequestResponse{start: 101, count: test.count, blocksFrom: server.PeerID(), bwb: []blocks.BlockWithROSidecars{{Block: first}, {Block: last}}}
			f.fetchPayloads(ctx, r, nil)
			require.NoError(t, r.err)
			require.Equal(t, 2, len(r.bwb))
			want := available
			if test.parentUnknown || test.wantStart > 100 {
				want = available[1:]
			}
			require.Equal(t, len(want), len(r.envelopes))
			for i := range want {
				require.DeepEqual(t, want[i].Proto(), r.envelopes[i].Proto())
			}
			require.Equal(t, int32(1), rangeRequests.Load())
			require.Equal(t, test.wantRootRequests, rootRequests.Load())
			columnBlocks, err := columnFetchBlocks(r.bwb, r.envelopes, slots.ToEpoch(lastSlot), func(root [32]byte) (blocks.ROBlock, bool) {
				return f.resolveBlock(ctx, root)
			})
			require.NoError(t, err)
			require.Equal(t, true, rootSet(columnBlocks)[last.Root()])
			if !test.parentUnknown && test.wantStart == 100 {
				require.Equal(t, true, rootSet(columnBlocks)[origin.Root()])
			}
			downscores, err := client.Peers().Scorers().BadResponsesScorer().Count(server.PeerID())
			require.NoError(t, err)
			require.Equal(t, 0, downscores)
		})
	}
}

func TestFetchPayloads_CappedForkRange(t *testing.T) {
	for _, test := range []struct {
		name         string
		firstSlot    primitives.Slot
		middleSlot   primitives.Slot
		lastSlot     primitives.Slot
		wantCount    uint64
		wantBlocks   int
		wantPayloads int
	}{
		{name: "full transition after capped range", firstSlot: 101, middleSlot: 250, lastSlot: 260, wantCount: 1, wantBlocks: 1},
		{name: "leading skipped slots", firstSlot: 250, middleSlot: 260, lastSlot: 270, wantCount: 21, wantBlocks: 3, wantPayloads: 1},
		{name: "block at exclusive end", firstSlot: 101, middleSlot: 229, lastSlot: 230, wantCount: 1, wantBlocks: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			f, client := newPayloadTestFetcher(t, 100)
			ancestorHash, middleHash := [32]byte{1}, [32]byte{3}
			origin := makeGloasBlockWithPayload(t, 100, [32]byte{}, ancestorHash, [32]byte{2})
			f.chain.(*mock.ChainService).BlockSlot = origin.Block().Slot()
			first := makeGloasBlockWithPayload(t, test.firstSlot, origin.Root(), ancestorHash, [32]byte{4})
			middle := makeGloasBlockWithPayload(t, test.middleSlot, first.Root(), ancestorHash, middleHash)
			last := makeGloasBlock(t, test.lastSlot, middle.Root(), middleHash)
			require.NoError(t, f.db.(db.Database).SaveBlock(ctx, origin.ReadOnlySignedBeaconBlock))
			envelope := makeEnvelopeForRoot(t, test.middleSlot, middle.Root(), middleHash, ancestorHash)
			server := p2ptest.NewTestP2P(t)
			client.Connect(server)
			var requests atomic.Int32
			server.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1), func(stream network.Stream) {
				defer func() { assert.NoError(t, stream.Close()) }()
				req := new(ethpb.ExecutionPayloadEnvelopesByRangeRequest)
				assert.NoError(t, server.Encoding().DecodeWithMaxLength(stream, req))
				requests.Add(1)
				assert.Equal(t, test.firstSlot, req.StartSlot)
				assert.Equal(t, test.wantCount, req.Count)
				if test.middleSlot >= req.StartSlot && test.middleSlot < req.StartSlot.Add(req.Count) {
					assert.NoError(t, prysmsync.WriteExecutionPayloadEnvelopeChunk(stream, server.Encoding(), envelope.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
				}
				assert.NoError(t, stream.CloseWrite())
			})
			fork, err := f.forkDataFromBlocks(ctx, server.PeerID(), []blocks.BlockWithROSidecars{{Block: first}, {Block: middle}, {Block: last}})
			require.NoError(t, err)
			require.Equal(t, test.wantBlocks, len(fork.bwb))
			require.Equal(t, first.Root(), fork.bwb[0].Block.Root())
			require.Equal(t, test.wantPayloads, len(fork.envelopes))
			if test.wantPayloads > 0 {
				require.DeepEqual(t, envelope.Proto(), fork.envelopes[0].Proto())
			}
			require.Equal(t, int32(1), requests.Load())
			downscores, err := client.Peers().Scorers().BadResponsesScorer().Count(server.PeerID())
			require.NoError(t, err)
			require.Equal(t, 0, downscores)
		})
	}
}
