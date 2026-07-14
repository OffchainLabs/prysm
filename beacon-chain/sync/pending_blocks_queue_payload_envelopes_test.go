package sync

import (
	"fmt"
	"sync"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
)

func makeSignedEnvelope(root [32]byte, slot primitives.Slot) *ethpb.SignedExecutionPayloadEnvelope {
	return &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    make([]byte, fieldparams.RootLength),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, fieldparams.RootLength),
				ReceiptsRoot:  make([]byte, fieldparams.RootLength),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, fieldparams.RootLength),
				BaseFeePerGas: make([]byte, fieldparams.RootLength),
				BlockHash:     make([]byte, fieldparams.RootLength),
				SlotNumber:    slot,
			},
			BeaconBlockRoot:       root[:],
			ParentBeaconBlockRoot: make([]byte, fieldparams.RootLength),
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
}

func setupGloasForkConfig(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()
}

func newEnvelopeFetchService(t *testing.T, p1 p2p.P2P) *Service {
	chain := &mock.ChainService{
		ValidatorsRoot:      [32]byte{},
		Genesis:             time.Now().Add(-time.Hour),
		FinalizedCheckPoint: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}
	r := &Service{
		cfg: &config{
			p2p:      p1,
			beaconDB: dbtest.SetupDB(t),
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		pendingPayloadEnvelopes:             make(map[[32]byte]map[uint64]*ethpb.SignedExecutionPayloadEnvelope),
		newExecutionPayloadEnvelopeVerifier: testNewExecutionPayloadEnvelopeVerifier(mockExecutionPayloadEnvelopeVerifier{}),
	}
	ctxMap, err := ContextByteVersionsForValRoot(chain.ValidatorsRoot)
	require.NoError(t, err)
	r.ctxMap = ctxMap
	return r
}

// Post-Gloas: an envelope is requested by root and queued even when no pending
// block for that root exists in the queue. This locks in the PR's removal of the
// per-block pendingBlockByRoot gate.
func TestFetchAndQueuePayloadEnvelopesForRoots_QueuesWithoutPendingBlock(t *testing.T) {
	setupGloasForkConfig(t)

	p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
	p1.Connect(p2)

	r := newEnvelopeFetchService(t, p1)

	root := [32]byte{0xAA}
	env := makeSignedEnvelope(root, 1)

	protocolID := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1)
	var wg sync.WaitGroup
	wg.Add(1)
	p2.SetStreamHandler(protocolID, func(stream network.Stream) {
		defer wg.Done()
		req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
		assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))
		require.Equal(t, 1, len(*req))
		assert.Equal(t, root, (*req)[0])
		assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), env))
		assert.NoError(t, stream.CloseWrite())
	})

	r.fetchAndQueuePayloadEnvelopesForRoots(t.Context(), p2.PeerID(), p2ptypes.BeaconBlockByRootsReq{root})

	if util.WaitTimeout(&wg, time.Second) {
		t.Fatal("Did not receive envelope-by-root request within 1 sec")
	}

	r.pendingEnvelopeLock.RLock()
	defer r.pendingEnvelopeLock.RUnlock()
	inner, ok := r.pendingPayloadEnvelopes[root]
	require.Equal(t, true, ok)
	require.Equal(t, 1, len(inner))
	assert.NotNil(t, inner[0])
}

// An envelope fetched by root with an invalid signature is dropped, not queued.
func TestQueuePendingPayloadEnvelopeFromRootRequest_RejectsBadSignature(t *testing.T) {
	setupGloasForkConfig(t)

	p1 := p2ptest.NewTestP2P(t)
	r := newEnvelopeFetchService(t, p1)
	r.newExecutionPayloadEnvelopeVerifier = testNewExecutionPayloadEnvelopeVerifier(
		mockExecutionPayloadEnvelopeVerifier{errSignature: errors.New("bad signature")},
	)

	root := [32]byte{0xBB}
	r.queuePendingPayloadEnvelopeFromRootRequest(t.Context(), nil, makeSignedEnvelope(root, 1))

	r.pendingEnvelopeLock.RLock()
	defer r.pendingEnvelopeLock.RUnlock()
	assert.Equal(t, 0, len(r.pendingPayloadEnvelopes))
}

// A far future slot must not defeat finalization based pruning by pinning memory.
func TestQueuePendingPayloadEnvelopeFromRootRequest_RejectsFarFutureSlot(t *testing.T) {
	setupGloasForkConfig(t)

	p1 := p2ptest.NewTestP2P(t)
	r := newEnvelopeFetchService(t, p1)

	root := [32]byte{0xDD}
	r.queuePendingPayloadEnvelopeFromRootRequest(t.Context(), nil, makeSignedEnvelope(root, 1_000_000))

	r.pendingEnvelopeLock.RLock()
	defer r.pendingEnvelopeLock.RUnlock()
	assert.Equal(t, 0, len(r.pendingPayloadEnvelopes))
}

func TestStorePendingPayloadEnvelope_Caps(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	r := &Service{pendingPayloadEnvelopes: make(map[[32]byte]map[uint64]*ethpb.SignedExecutionPayloadEnvelope)}

	for i := 0; i < maxPendingPayloadRoots; i++ {
		env := makeSignedEnvelope([32]byte{byte(i), byte(i >> 8)}, 1)
		stored, _ := r.storePendingPayloadEnvelope(env, false)
		require.Equal(t, true, stored)
	}
	require.Equal(t, maxPendingPayloadRoots, len(r.pendingPayloadEnvelopes))

	overflow := makeSignedEnvelope([32]byte{0xFF, 0xFF, 0xFF}, 1)
	stored, _ := r.storePendingPayloadEnvelope(overflow, false)
	assert.Equal(t, false, stored)

	selfBuild := makeSignedEnvelope([32]byte{0xFE, 0xFE, 0xFE}, 1)
	selfBuild.Message.BuilderIndex = primitives.BuilderIndex(params.BeaconConfig().BuilderIndexSelfBuild)
	stored, _ = r.storePendingPayloadEnvelope(selfBuild, true)
	assert.Equal(t, true, stored)
}

func TestStorePendingPayloadEnvelope_BuildersPerRootCap(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	r := &Service{pendingPayloadEnvelopes: make(map[[32]byte]map[uint64]*ethpb.SignedExecutionPayloadEnvelope)}

	root := [32]byte{0x01}
	for i := 0; i < maxPendingBuildersPerRoot; i++ {
		env := makeSignedEnvelope(root, 1)
		env.Message.BuilderIndex = primitives.BuilderIndex(i)
		stored, _ := r.storePendingPayloadEnvelope(env, false)
		require.Equal(t, true, stored)
	}
	overflow := makeSignedEnvelope(root, 1)
	overflow.Message.BuilderIndex = primitives.BuilderIndex(maxPendingBuildersPerRoot)
	stored, _ := r.storePendingPayloadEnvelope(overflow, false)
	assert.Equal(t, false, stored)
}

// Pre-Gloas: the chain-level CurrentSlot() < gloasStartSlot gate short-circuits
// before any request is sent.
func TestFetchAndQueuePayloadEnvelopesForRoots_PreGloasNoRequest(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 1_000_000 // far enough ahead that CurrentSlot() is well before it
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
	p1.Connect(p2)

	r := newEnvelopeFetchService(t, p1)

	protocolID := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1)
	p2.SetStreamHandler(protocolID, func(network.Stream) {
		t.Error("envelope request should not be sent before Gloas")
	})

	r.fetchAndQueuePayloadEnvelopesForRoots(t.Context(), p2.PeerID(), p2ptypes.BeaconBlockByRootsReq{{0xAA}})

	assert.Equal(t, 0, len(r.pendingPayloadEnvelopes))
}

// Post-Gloas: a root whose envelope is already in the DB is filtered out, so no
// request is sent for it.
func TestFetchAndQueuePayloadEnvelopesForRoots_SkipsRootAlreadyInDB(t *testing.T) {
	setupGloasForkConfig(t)

	p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
	p1.Connect(p2)

	r := newEnvelopeFetchService(t, p1)

	root := bytesutil.ToBytes32([]byte{0xCC})
	env := makeSignedEnvelope(root, 1)
	require.NoError(t, r.cfg.beaconDB.SaveExecutionPayloadEnvelope(t.Context(), env))

	protocolID := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1)
	p2.SetStreamHandler(protocolID, func(network.Stream) {
		t.Error("no request should be sent for a root already present in the DB")
	})

	r.fetchAndQueuePayloadEnvelopesForRoots(t.Context(), p2.PeerID(), p2ptypes.BeaconBlockByRootsReq{root})

	assert.Equal(t, 0, len(r.pendingPayloadEnvelopes))
}
