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
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
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
	env := &ethpb.SignedExecutionPayloadEnvelope{
		Signature: make([]byte, fieldparams.BLSSignatureLength),
		Message: &ethpb.ExecutionPayloadEnvelope{
			BeaconBlockRoot:       make([]byte, fieldparams.RootLength),
			ParentBeaconBlockRoot: make([]byte, fieldparams.RootLength),
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    parentHash[:],
				FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:     make([]byte, fieldparams.RootLength),
				ReceiptsRoot:  make([]byte, fieldparams.RootLength),
				LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:    make([]byte, fieldparams.RootLength),
				BaseFeePerGas: make([]byte, fieldparams.RootLength),
				BlockHash:     blockHash[:],
				SlotNumber:    slot,
			},
		},
	}
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)
	return wrapped
}

func makePayloadEnvelopeForRoot(t *testing.T, slot primitives.Slot, root, blockHash, parentHash [32]byte) interfaces.ROSignedExecutionPayloadEnvelope {
	t.Helper()
	proto := makeEnvelope(t, slot, blockHash, parentHash).Proto().(*ethpb.SignedExecutionPayloadEnvelope)
	proto.Message.BeaconBlockRoot = root[:]
	envelope, err := blocks.WrappedROSignedExecutionPayloadEnvelope(proto)
	require.NoError(t, err)
	return envelope
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

func TestBlockBuiltOnEnvelope(t *testing.T) {
	blockHash := [32]byte{0xaa}
	parentHash := [32]byte{0xbb}

	t.Run("matching execution parent hash returns true", func(t *testing.T) {
		env := makeEnvelope(t, 10, blockHash, [32]byte{})
		blk := makeGloasBlock(t, 11, [32]byte{}, blockHash)
		full, err := blocks.BlockBuiltOnEnvelope(env, blk)
		require.NoError(t, err)
		require.Equal(t, true, full)
	})

	t.Run("different execution parent hash returns false", func(t *testing.T) {
		env := makeEnvelope(t, 10, blockHash, [32]byte{})
		blk := makeGloasBlock(t, 11, [32]byte{}, parentHash)
		full, err := blocks.BlockBuiltOnEnvelope(env, blk)
		require.NoError(t, err)
		require.Equal(t, false, full)
	})
}

func TestBlockBuiltOnParentEnvelope(t *testing.T) {
	blockHash := [32]byte{0xaa}
	parentRoot := [32]byte{0x01}

	t.Run("matching beacon parent root and execution hash returns true", func(t *testing.T) {
		blk := makeGloasBlock(t, 11, parentRoot, blockHash)
		env := makePayloadEnvelopeForRoot(t, 10, parentRoot, blockHash, [32]byte{})
		full, err := blocks.BlockBuiltOnParentEnvelope(env, blk)
		require.NoError(t, err)
		require.Equal(t, true, full)
	})

	t.Run("ancestor root with matching execution hash returns false", func(t *testing.T) {
		blk := makeGloasBlock(t, 11, parentRoot, blockHash)
		env := makePayloadEnvelopeForRoot(t, 9, [32]byte{0x02}, blockHash, [32]byte{})
		full, err := blocks.BlockBuiltOnParentEnvelope(env, blk)
		require.NoError(t, err)
		require.Equal(t, false, full)
	})

	t.Run("matching beacon parent root with different execution hash returns false", func(t *testing.T) {
		blk := makeGloasBlock(t, 11, parentRoot, [32]byte{0xbb})
		env := makePayloadEnvelopeForRoot(t, 10, parentRoot, blockHash, [32]byte{})
		full, err := blocks.BlockBuiltOnParentEnvelope(env, blk)
		require.NoError(t, err)
		require.Equal(t, false, full)
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
			makePayloadEnvelopeForRoot(t, 10, anchor.Root(), [32]byte{3}, [32]byte{2}),
		},
	}
	f := &blocksFetcher{}
	f.validatePayloadsForImport(r, 2)
	require.NoError(t, r.err)
	require.Equal(t, 3, len(r.bwb))
	require.Equal(t, child.Root(), r.bwb[2].Block.Root())
	require.Equal(t, 1, len(r.envelopes))
}

func TestFetchPayloads_RequiredParent(t *testing.T) {
	parentHash, blockHash := [32]byte{1}, [32]byte{2}
	parent := makeGloasBlockWithPayload(t, 10, [32]byte{}, parentHash, blockHash)
	child := makeGloasBlock(t, 14, parent.Root(), blockHash)
	emptyChild := makeGloasBlock(t, 14, parent.Root(), parentHash)
	older := makeGloasBlockWithPayload(t, 8, [32]byte{}, [32]byte{3}, parentHash)
	envelope := makePayloadEnvelopeForRoot(t, 10, parent.Root(), blockHash, parentHash)
	childEnvelope := makePayloadEnvelopeForRoot(t, 14, child.Root(), [32]byte{4}, blockHash)
	genesis := makeGloasBlockWithPayload(t, 0, [32]byte{}, parentHash, blockHash)
	genesisChild := makeGloasBlock(t, 1, genesis.Root(), blockHash)
	tests := []struct {
		name          string
		blocks        []blocks.BlockWithROSidecars
		head          primitives.Slot
		rangePayload  interfaces.ROSignedExecutionPayloadEnvelope
		missingParent bool
		unknownFork   bool
		rootRequests  int32
		wantPayloads  int
		wantErr       string
	}{
		{name: "genesis full child needs no envelope", blocks: []blocks.BlockWithROSidecars{{Block: genesis}, {Block: genesisChild}}, head: 0},
		{name: "skipped slots resolve database parent", blocks: []blocks.BlockWithROSidecars{{Block: child}}, head: 10, rootRequests: 1, wantPayloads: 1},
		{name: "missing full origin rejects batch", blocks: []blocks.BlockWithROSidecars{{Block: child}}, head: 10, missingParent: true, rootRequests: 1},
		{name: "known origin in batch", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10, rootRequests: 1, wantPayloads: 1},
		{name: "unknown fork below head needs parent", blocks: []blocks.BlockWithROSidecars{{Block: child}}, head: 16, unknownFork: true, rootRequests: 1, wantPayloads: 1},
		{name: "unknown fork follows known anchor below head", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 16, unknownFork: true, rootRequests: 1, wantPayloads: 1},
		{name: "old imported transitions need no envelopes", blocks: []blocks.BlockWithROSidecars{{Block: older}, {Block: parent}, {Block: child}}, head: 10, rootRequests: 1, wantPayloads: 1},
		{name: "recovered parent follows an older range payload", blocks: []blocks.BlockWithROSidecars{{Block: older}, {Block: parent}, {Block: child}}, head: 10,
			rangePayload: makePayloadEnvelopeForRoot(t, 8, older.Root(), parentHash, [32]byte{3}), rootRequests: 1, wantPayloads: 1},
		{name: "empty withheld origin", blocks: []blocks.BlockWithROSidecars{{Block: emptyChild}}, head: 10},
		{name: "parent already returned by range", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10, rangePayload: envelope, wantPayloads: 1},
		{name: "wrong range parent hash rejects batch", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10,
			rangePayload: makePayloadEnvelopeForRoot(t, 10, parent.Root(), [32]byte{99}, parentHash), wantPayloads: 1, wantErr: "parent payload envelope does not match block"},
		{name: "wrong range parent slot rejects batch", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 10,
			rangePayload: makePayloadEnvelopeForRoot(t, 9, parent.Root(), blockHash, parentHash), wantPayloads: 1, wantErr: "parent payload envelope does not match block"},
		{name: "recovered parent precedes child payload", blocks: []blocks.BlockWithROSidecars{{Block: child}}, head: 10, rangePayload: childEnvelope, rootRequests: 1, wantPayloads: 2},
		{name: "all known needs no parent", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 14},
		{name: "all known retains new head payload", blocks: []blocks.BlockWithROSidecars{{Block: parent}, {Block: child}}, head: 14, rangePayload: childEnvelope, wantPayloads: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, client := newPayloadTestFetcher(t, tt.head)
			require.NoError(t, f.db.(db.Database).SaveBlock(t.Context(), parent.ReadOnlySignedBeaconBlock))
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
			} else if tt.missingParent {
				require.ErrorContains(t, "missing payload envelope for FULL parent", r.err)
			} else {
				require.NoError(t, r.err)
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

func TestFetchParentPayloadFromPeers(t *testing.T) {
	parentHash, blockHash := [32]byte{1}, [32]byte{2}
	parent := makeGloasBlockWithPayload(t, 10, [32]byte{}, parentHash, blockHash)
	child := makeGloasBlock(t, 14, parent.Root(), blockHash)
	valid := makePayloadEnvelopeForRoot(t, 10, parent.Root(), blockHash, parentHash)
	for _, response := range []string{"unavailable", "missing", "wrong root", "wrong hash", "wrong slot"} {
		t.Run(response, func(t *testing.T) {
			f, client := newPayloadTestFetcher(t, 10)
			bad, good := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
			client.Connect(bad)
			client.Connect(good)
			var failedRequests atomic.Int32
			protocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1)
			if response != "unavailable" {
				bad.SetStreamHandler(protocol, func(stream network.Stream) {
					defer func() { assert.NoError(t, stream.Close()) }()
					req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
					assert.NoError(t, bad.Encoding().DecodeWithMaxLength(stream, req))
					failedRequests.Add(1)
					root, hash, slot := parent.Root(), blockHash, primitives.Slot(10)
					switch response {
					case "wrong root":
						root = [32]byte{99}
					case "wrong hash":
						hash = [32]byte{99}
					case "wrong slot":
						slot = 9
					}
					if response != "missing" {
						envelope := makePayloadEnvelopeForRoot(t, slot, root, hash, parentHash)
						assert.NoError(t, prysmsync.WriteExecutionPayloadEnvelopeChunk(stream, bad.Encoding(), envelope.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
					}
					assert.NoError(t, stream.CloseWrite())
				})
			}
			good.SetStreamHandler(protocol, func(stream network.Stream) {
				defer func() { assert.NoError(t, stream.Close()) }()
				req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
				assert.NoError(t, good.Encoding().DecodeWithMaxLength(stream, req))
				assert.NoError(t, prysmsync.WriteExecutionPayloadEnvelopeChunk(stream, good.Encoding(), valid.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
				assert.NoError(t, stream.CloseWrite())
			})
			_, _, err := f.fetchParentPayloadFromPeers(t.Context(), parent, child, bad.PeerID(), nil)
			require.ErrorContains(t, "missing payload envelope for FULL parent", err)
			envelope, provider, err := f.fetchParentPayloadFromPeers(t.Context(), parent, child, bad.PeerID(), []peer.ID{bad.PeerID(), good.PeerID()})
			require.NoError(t, err)
			require.Equal(t, good.PeerID(), provider)
			matches, err := blocks.BlockBuiltOnParentEnvelope(envelope, child)
			require.NoError(t, err)
			require.Equal(t, true, matches)
			if response != "unavailable" {
				require.Equal(t, int32(2), failedRequests.Load())
			}
		})
	}
}

func TestFetchPayloads_PrefetchedBatchRecoversParentAfterItBecomesKnown(t *testing.T) {
	parentHash := [32]byte{1}
	origin := makeGloasBlockWithPayload(t, 10, [32]byte{}, parentHash, [32]byte{2})
	parent := makeGloasBlockWithPayload(t, 14, origin.Root(), parentHash, [32]byte{3})
	child := makeGloasBlock(t, 18, parent.Root(), [32]byte{3})
	parentEnvelope := makePayloadEnvelopeForRoot(t, 14, parent.Root(), [32]byte{3}, parentHash)
	f, client := newPayloadTestFetcher(t, 10)
	store := f.db.(db.Database)
	require.NoError(t, store.SaveBlock(t.Context(), origin.ReadOnlySignedBeaconBlock))
	server := p2ptest.NewTestP2P(t)
	client.Connect(server)
	server.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1), func(stream network.Stream) {
		defer func() { assert.NoError(t, stream.Close()) }()
		req := new(ethpb.ExecutionPayloadEnvelopesByRangeRequest)
		assert.NoError(t, server.Encoding().DecodeWithMaxLength(stream, req))
		assert.NoError(t, stream.CloseWrite())
	})
	var rootRequests atomic.Int32
	server.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1), func(stream network.Stream) {
		defer func() { assert.NoError(t, stream.Close()) }()
		req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
		assert.NoError(t, server.Encoding().DecodeWithMaxLength(stream, req))
		assert.DeepEqual(t, p2ptypes.ExecutionPayloadEnvelopesByRootReq{parent.Root()}, *req)
		rootRequests.Add(1)
		assert.NoError(t, prysmsync.WriteExecutionPayloadEnvelopeChunk(stream, server.Encoding(), parentEnvelope.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
		assert.NoError(t, stream.CloseWrite())
	})

	firstBatch := &fetchRequestResponse{start: 14, count: 4, blocksFrom: server.PeerID(), bwb: []blocks.BlockWithROSidecars{{Block: parent}}}
	f.fetchPayloads(t.Context(), firstBatch, nil)
	require.NoError(t, firstBatch.err)
	prefetched := &fetchRequestResponse{start: 18, count: 4, blocksFrom: server.PeerID(), bwb: []blocks.BlockWithROSidecars{{Block: child}}}
	f.fetchPayloads(t.Context(), prefetched, nil)
	require.NoError(t, prefetched.err)
	require.Equal(t, 0, len(prefetched.envelopes))
	require.Equal(t, int32(0), rootRequests.Load())

	// Simulate the database and head visibility established by importing the first batch.
	require.NoError(t, store.SaveBlock(t.Context(), parent.ReadOnlySignedBeaconBlock))
	*f.chain.(*mock.ChainService).MockHeadSlot = parent.Block().Slot()
	retry := &fetchRequestResponse{start: 18, count: 4, blocksFrom: server.PeerID(), bwb: []blocks.BlockWithROSidecars{{Block: child}}}
	f.fetchPayloads(t.Context(), retry, nil)
	require.NoError(t, retry.err)
	require.Equal(t, int32(1), rootRequests.Load())
	require.Equal(t, 1, len(retry.envelopes))
	matches, err := blocks.BlockBuiltOnParentEnvelope(retry.envelopes[0], child)
	require.NoError(t, err)
	require.Equal(t, true, matches)
}
