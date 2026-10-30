package sync

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func newChunkTestService(t *testing.T, blockRoot [32]byte) *Service {
	t.Helper()
	db := dbtest.SetupDB(t)
	// Genesis one slot ago so the wall clock sits in slot 1.
	genesis := time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0)
	chain := &mock.ChainService{
		Genesis:            genesis,
		DB:                 db,
		InitSyncBlockRoots: map[[32]byte]bool{blockRoot: true},
	}
	return &Service{
		ctx:                      context.Background(),
		chainStarted:             &atomic.Bool{},
		pendingPayloadEnvelopes:  make(map[[32]byte]map[uint64]*ethpb.SignedExecutionPayloadEnvelope),
		pendingHezeEnvelopes:     make(map[[32]byte]interfaces.ROSignedExecutionPayloadEnvelope),
		pendingPayloadChunks:     make(map[[32]byte]*pendingChunkEntry),
		payloadChunkPool:         newPayloadChunkPool(),
		payloadChunkCommitments:  newSlotAwareCache(16),
		seenPayloadChunkCache:    newSlotAwareCache(1024),
		seenPayloadEnvelopeCache: lruwrpr.New(10),
		badBlockCache:            lruwrpr.New(10),
		cfg: &config{
			chain:    chain,
			beaconDB: db,
			clock:    startup.NewClock(genesis, chain.ValidatorsRoot),
		},
	}
}

// A chunk set that completes before its block binding is known must not be
// lost or permanently misrouted: the unauthenticated beacon_block_root hint is
// only a hint, and the authoritative binding retries the import.
func TestReconstructedEnvelopeBindingRetry(t *testing.T) {
	const slot = primitives.Slot(1)
	builderIdx := primitives.BuilderIndex(7)
	blockRoot := [32]byte{0xAB, 0xCD}
	blockHash := [32]byte{0xBB}
	envelope := testSignedExecutionPayloadEnvelopeHeze(t, slot, builderIdx, blockHash).Message

	waitSeen := func(t *testing.T, s *Service) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if s.hasSeenPayloadEnvelope(blockRoot, builderIdx) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("timed out waiting for the reconstructed envelope import")
	}

	t.Run("zero hint waits for the binding", func(t *testing.T) {
		s := newChunkTestService(t, blockRoot)
		chunks, chunksRoot, err := heze.BuildPayloadChunkMessages(envelope, slot, [32]byte{})
		require.NoError(t, err)

		// The bid was seen on gossip only: commitment known, binding unknown.
		s.registerPayloadChunkCommitment(slot, chunksRoot, [32]byte{})
		for _, c := range chunks {
			s.acceptVerifiedPayloadChunk(s.ctx, c)
		}
		// Complete, but with no binding and no hint nothing can be imported yet.
		_, done := s.payloadChunkPool.completedChunks(chunksRoot, slot)
		require.Equal(t, true, done)
		require.Equal(t, false, s.hasSeenPayloadEnvelope(blockRoot, builderIdx))

		// The block arrives: the binding registers and the retry imports the set.
		s.registerPayloadChunkCommitment(slot, chunksRoot, blockRoot)
		waitSeen(t, s)
	})

	t.Run("forged hint is corrected by the binding", func(t *testing.T) {
		s := newChunkTestService(t, blockRoot)
		forged := [32]byte{0xFF, 0xEE}
		chunks, chunksRoot, err := heze.BuildPayloadChunkMessages(envelope, slot, forged)
		require.NoError(t, err)

		s.registerPayloadChunkCommitment(slot, chunksRoot, [32]byte{})
		for _, c := range chunks {
			s.acceptVerifiedPayloadChunk(s.ctx, c)
		}
		// Misrouted: parked under the forged root, nothing imported, and the
		// pool entry is not marked imported (hint routing is not authoritative).
		require.Equal(t, false, s.hasSeenPayloadEnvelope(blockRoot, builderIdx))
		s.pendingHezeEnvelopesLock.Lock()
		_, parked := s.pendingHezeEnvelopes[forged]
		s.pendingHezeEnvelopesLock.Unlock()
		require.Equal(t, true, parked)
		_, done := s.payloadChunkPool.completedChunks(chunksRoot, slot)
		require.Equal(t, true, done)

		s.registerPayloadChunkCommitment(slot, chunksRoot, blockRoot)
		waitSeen(t, s)
	})

	t.Run("authoritative routing is not retried", func(t *testing.T) {
		s := newChunkTestService(t, blockRoot)
		chunks, chunksRoot, err := heze.BuildPayloadChunkMessages(envelope, slot, blockRoot)
		require.NoError(t, err)

		// Binding known before completion: import happens inline and the pool
		// entry is consumed for retry purposes.
		s.registerPayloadChunkCommitment(slot, chunksRoot, blockRoot)
		for _, c := range chunks {
			s.acceptVerifiedPayloadChunk(s.ctx, c)
		}
		require.Equal(t, true, s.hasSeenPayloadEnvelope(blockRoot, builderIdx))
		_, done := s.payloadChunkPool.completedChunks(chunksRoot, slot)
		require.Equal(t, false, done)
	})
}

// Losing bids must not be able to starve the winning bid's reconstruction:
// bound entries displace unbound partial sets, and a rejected insert rolls
// back the first-seen marker so a later delivery still has a path in.
func TestPayloadChunkPoolEviction(t *testing.T) {
	mkChunk := func(root [32]byte, idx uint64) *ethpb.PayloadChunk {
		return &ethpb.PayloadChunk{
			PayloadChunksRoot: root[:],
			Index:             idx,
			Chunk:             []byte{0x1},
			Slot:              1,
			BeaconBlockRoot:   make([]byte, 32),
		}
	}

	t.Run("bound newcomer evicts the weakest unbound entry", func(t *testing.T) {
		p := newPayloadChunkPool()
		for i := range maxChunkPoolRoots {
			root := [32]byte{0x10, byte(i)}
			accepted, _, _, _, _, _ := p.insert(mkChunk(root, 0), false)
			require.Equal(t, true, accepted)
		}
		winner := [32]byte{0x99}
		accepted, _, _, _, _, _ := p.insert(mkChunk(winner, 0), true)
		require.Equal(t, true, accepted)
		require.Equal(t, maxChunkPoolRoots, len(p.entries))
		require.NotNil(t, p.entries[winner])
	})

	t.Run("unbound newcomer cannot displace bound entries", func(t *testing.T) {
		p := newPayloadChunkPool()
		for i := range maxChunkPoolRoots {
			root := [32]byte{0x20, byte(i)}
			accepted, _, _, _, _, _ := p.insert(mkChunk(root, 0), true)
			require.Equal(t, true, accepted)
		}
		loser := [32]byte{0x98}
		accepted, _, _, _, _, _ := p.insert(mkChunk(loser, 0), false)
		require.Equal(t, false, accepted)
	})

	t.Run("rejected insert rolls back the first-seen marker", func(t *testing.T) {
		s := newChunkTestService(t, [32]byte{0xAB})
		for i := range maxChunkPoolRoots {
			root := [32]byte{0x30, byte(i)}
			accepted, _, _, _, _, _ := s.payloadChunkPool.insert(mkChunk(root, 0), true)
			require.Equal(t, true, accepted)
		}
		root := [32]byte{0x97}
		c := mkChunk(root, 3)
		// Gossip validation marks seen before the subscriber runs.
		s.setSeenPayloadChunk(1, root, c.Index)
		s.acceptVerifiedPayloadChunk(s.ctx, c)
		require.Equal(t, false, s.hasSeenPayloadChunk(root, c.Index))
	})
}

// testSignedExecutionPayloadEnvelopeHeze lifts the Gloas envelope fixture into
// the Heze container, which carries no beacon block root (EIP-8411).
func testSignedExecutionPayloadEnvelopeHeze(t *testing.T, slot primitives.Slot, builderIdx primitives.BuilderIndex, blockHash [32]byte) *ethpb.SignedExecutionPayloadEnvelopeHeze {
	t.Helper()
	gloasSigned := testSignedExecutionPayloadEnvelope(t, slot, builderIdx, [32]byte{}, blockHash)
	return &ethpb.SignedExecutionPayloadEnvelopeHeze{
		Message: &ethpb.ExecutionPayloadEnvelopeHeze{
			Payload:               gloasSigned.Message.Payload,
			ExecutionRequests:     gloasSigned.Message.ExecutionRequests,
			BuilderIndex:          gloasSigned.Message.BuilderIndex,
			ParentBeaconBlockRoot: gloasSigned.Message.ParentBeaconBlockRoot,
		},
		Signature: gloasSigned.Signature,
	}
}

// Eviction frees bytes, so the first-seen markers for those bytes must be
// cleared too. Otherwise the discarded chunks are refused on re-offer and the
// commitment can never be reconstructed again for the rest of its slot.
func TestPayloadChunkPool_EvictionClearsSeenMarkers(t *testing.T) {
	mkChunk := func(root [32]byte, idx uint64) *ethpb.PayloadChunk {
		return &ethpb.PayloadChunk{
			PayloadChunksRoot: root[:],
			Index:             idx,
			Chunk:             []byte{0x1},
			Slot:              1,
		}
	}
	s := newChunkTestService(t, [32]byte{0xAB})
	// Fill the pool with unbound entries, marking each chunk seen the way
	// gossip validation does.
	for i := range maxChunkPoolRoots {
		root := [32]byte{0x40, byte(i)}
		c := mkChunk(root, 0)
		s.setSeenPayloadChunk(c.Slot, root, c.Index)
		s.acceptVerifiedPayloadChunk(t.Context(), c)
	}
	victim := [32]byte{0x40, 0}
	require.Equal(t, true, s.hasSeenPayloadChunk(victim, 0))

	// A bound newcomer forces an eviction.
	newcomer := mkChunk([32]byte{0x41}, 0)
	s.registerPayloadChunkCommitment(newcomer.Slot, [32]byte{0x41}, [32]byte{0xCC})
	s.setSeenPayloadChunk(newcomer.Slot, [32]byte{0x41}, newcomer.Index)
	s.acceptVerifiedPayloadChunk(t.Context(), newcomer)

	// Exactly one unbound entry lost its bytes; that one must no longer be
	// marked seen, so its chunks can be offered again.
	cleared := 0
	for i := range maxChunkPoolRoots {
		if !s.hasSeenPayloadChunk([32]byte{0x40, byte(i)}, 0) {
			cleared++
		}
	}
	require.Equal(t, 1, cleared)
}
