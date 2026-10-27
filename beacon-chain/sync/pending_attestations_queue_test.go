package sync

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/ethereum/go-ethereum/p2p/enr"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

var verifierLimit = 1000

// blocksByRootRecorder serves no blocks on p and reports every root requested of it.
func blocksByRootRecorder(p *p2ptest.TestP2P) <-chan [32]byte {
	roots := make(chan [32]byte, 64)
	handler := func(stream network.Stream) {
		req := new(p2ptypes.BeaconBlockByRootsReq)
		if err := p.Encoding().DecodeWithMaxLength(stream, req); err == nil {
			for _, r := range *req {
				roots <- r
			}
		}
		_ = stream.Close()
	}
	for _, topic := range []string{p2p.RPCBlocksByRootTopicV1, p2p.RPCBlocksByRootTopicV2} {
		p.BHost.SetStreamHandler(protocol.ID(topic+p.Encoding().ProtocolSuffix()), handler)
	}
	return roots
}

// envelopesByRootRecorder serves no envelopes on p and reports every root requested of it.
func envelopesByRootRecorder(p *p2ptest.TestP2P) <-chan [32]byte {
	roots := make(chan [32]byte, 64)
	p.BHost.SetStreamHandler(protocol.ID(p2p.RPCExecutionPayloadEnvelopesByRootTopicV1+p.Encoding().ProtocolSuffix()), func(stream network.Stream) {
		req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
		if err := p.Encoding().DecodeWithMaxLength(stream, req); err == nil {
			for _, r := range *req {
				roots <- r
			}
		}
		_ = stream.Close()
	})
	return roots
}

// pendingAttService returns a service that knows only the returned block, so pending attestations for
// any other root need a request.
func pendingAttService(t *testing.T, p *p2ptest.TestP2P) (*Service, [32]byte) {
	db := dbtest.SetupDB(t)
	blockA := util.NewBeaconBlock()
	util.SaveBlock(t, t.Context(), db, blockA)
	rootA, err := blockA.Block.HashTreeRoot()
	require.NoError(t, err)
	stateA, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(t.Context(), stateA, rootA))

	chain := &mock.ChainService{
		Genesis:             prysmTime.Now(),
		FinalizedCheckPoint: &ethpb.Checkpoint{},
		ForkchoiceRoots:     map[[32]byte]bool{rootA: true},
	}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := &Service{
		ctx:                  ctx,
		cfg:                  &config{p2p: p, beaconDB: db, chain: chain, clock: startup.NewClock(chain.Genesis, chain.ValidatorsRoot)},
		blkRootToPendingAtts: make(map[[32]byte][]any),
		seenPendingBlocks:    make(map[[32]byte]bool),
		chainStarted:         &atomic.Bool{},
	}
	r.initCaches()
	return r, rootA
}

func pendingAttFor(root [32]byte, slot primitives.Slot) *ethpb.Attestation {
	return &ethpb.Attestation{Data: &ethpb.AttestationData{
		Slot:            slot,
		BeaconBlockRoot: root[:],
		Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
	}}
}

func TestProcessPendingAtts_RequestsBlockFromSender(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	sender := p2ptest.NewTestP2P(t)
	other := p2ptest.NewTestP2P(t)
	p1.Connect(sender)
	p1.Connect(other)
	senderRoots := blocksByRootRecorder(sender)
	otherRoots := blocksByRootRecorder(other)
	// Make other a best peer, which a random batch request would have picked.
	p1.Peers().Add(new(enr.Record), other.PeerID(), nil, network.DirOutbound)
	p1.Peers().SetConnectionState(other.PeerID(), peers.Connected)
	p1.Peers().SetChainState(other.PeerID(), &ethpb.StatusV2{})

	r, rootA := pendingAttService(t, p1)
	rootB := [32]byte{'B'}
	r.savePendingAtt(sender.PeerID(), pendingAttFor(rootB, 1))

	require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))

	select {
	case got := <-senderRoots:
		require.Equal(t, rootB, got)
	case <-time.After(5 * time.Second):
		t.Fatal("block was not requested from the sender")
	}
	select {
	case got := <-otherRoots:
		t.Fatalf("block %#x was requested from a peer that sent no attestation for it", got)
	case <-time.After(200 * time.Millisecond):
	}
	require.Eventually(t, func() bool {
		r.pendingAttsLock.RLock()
		defer r.pendingAttsLock.RUnlock()
		return len(r.pendingAttRequestPeers) == 0
	}, 5*time.Second, 10*time.Millisecond, "in-flight request was not released")
}

func TestProcessPendingAtts_RequestsEnvelopeWithBlock(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	p1 := p2ptest.NewTestP2P(t)
	sender := p2ptest.NewTestP2P(t)
	p1.Connect(sender)
	blockRoots := blocksByRootRecorder(sender)
	envelopeRoots := envelopesByRootRecorder(sender)

	r, rootA := pendingAttService(t, p1)
	ctxMap, err := ContextByteVersionsForValRoot(r.cfg.chain.(*mock.ChainService).ValidatorsRoot)
	require.NoError(t, err)
	r.ctxMap = ctxMap
	r.pendingPayloadEnvelopes = make(map[[32]byte]map[uint64]*ethpb.SignedExecutionPayloadEnvelope)
	rootB := [32]byte{'B'}
	r.savePendingAtt(sender.PeerID(), pendingAttFor(rootB, 1))

	require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))

	for name, roots := range map[string]<-chan [32]byte{"block": blockRoots, "envelope": envelopeRoots} {
		select {
		case got := <-roots:
			require.Equal(t, rootB, got, name)
		case <-time.After(5 * time.Second):
			t.Fatalf("%s was not requested from the sender", name)
		}
	}
}

func TestProcessPendingAtts_BatchesRequestsPerSender(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	sender := p2ptest.NewTestP2P(t)
	p1.Connect(sender)
	senderRoots := blocksByRootRecorder(sender)

	r, rootA := pendingAttService(t, p1)
	rootB, rootC := [32]byte{'B'}, [32]byte{'C'}
	r.savePendingAtt(sender.PeerID(), pendingAttFor(rootB, 1))
	r.savePendingAtt(sender.PeerID(), pendingAttFor(rootC, 1))

	require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))

	got := make(map[[32]byte]bool)
	for range 2 {
		select {
		case root := <-senderRoots:
			got[root] = true
		case <-time.After(5 * time.Second):
			t.Fatal("blocks were not requested from the sender")
		}
	}
	require.DeepEqual(t, map[[32]byte]bool{rootB: true, rootC: true}, got)
}

func TestProcessPendingAtts_OneRequestInFlightPerPeer(t *testing.T) {
	t.Run("busy sole sender is skipped", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		sender := p2ptest.NewTestP2P(t)
		p1.Connect(sender)
		senderRoots := blocksByRootRecorder(sender)

		r, rootA := pendingAttService(t, p1)
		rootB := [32]byte{'B'}
		r.savePendingAtt(sender.PeerID(), pendingAttFor(rootB, 1))
		r.pendingAttRequestPeers[sender.PeerID()] = true

		require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))
		select {
		case got := <-senderRoots:
			t.Fatalf("unexpected request for block %#x", got)
		case <-time.After(200 * time.Millisecond):
		}
	})

	t.Run("busy sender falls back to another sender", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		busy := p2ptest.NewTestP2P(t)
		idle := p2ptest.NewTestP2P(t)
		p1.Connect(busy)
		p1.Connect(idle)
		busyRoots := blocksByRootRecorder(busy)
		idleRoots := blocksByRootRecorder(idle)

		r, rootA := pendingAttService(t, p1)
		rootB := [32]byte{'B'}
		r.savePendingAtt(busy.PeerID(), pendingAttFor(rootB, 1))
		r.savePendingAtt(idle.PeerID(), pendingAttFor(rootB, 2))
		r.pendingAttRequestPeers[busy.PeerID()] = true

		require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))
		select {
		case got := <-idleRoots:
			require.Equal(t, rootB, got)
		case <-time.After(5 * time.Second):
			t.Fatal("block was not requested from the idle sender")
		}
		select {
		case got := <-busyRoots:
			t.Fatalf("block %#x was requested from a peer with a request in flight", got)
		case <-time.After(200 * time.Millisecond):
		}
	})
}

func TestProcessPendingAtts_SkipsDisconnectedSender(t *testing.T) {
	t.Run("sole sender disconnected", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		gone := p2ptest.NewTestP2P(t)
		p1.Connect(gone)
		goneRoots := blocksByRootRecorder(gone)

		r, rootA := pendingAttService(t, p1)
		rootB := [32]byte{'B'}
		r.savePendingAtt(gone.PeerID(), pendingAttFor(rootB, 1))
		require.NoError(t, p1.BHost.Network().ClosePeer(gone.PeerID()))

		require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))
		select {
		case got := <-goneRoots:
			t.Fatalf("block %#x was requested from a disconnected peer", got)
		case <-time.After(200 * time.Millisecond):
		}

		r.pendingAttsLock.RLock()
		defer r.pendingAttsLock.RUnlock()
		require.Equal(t, 0, len(r.pendingAttSenders))
		require.Equal(t, 0, len(r.pendingAttRequestPeers))

		// The disconnected peer is not downscored.
		count, err := p1.Peers().Scorers().BadResponsesScorer().Count(gone.PeerID())
		if err == nil {
			require.Equal(t, 0, count)
		}
	})

	t.Run("connected sender asked instead", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		gone := p2ptest.NewTestP2P(t)
		connected := p2ptest.NewTestP2P(t)
		p1.Connect(gone)
		p1.Connect(connected)
		goneRoots := blocksByRootRecorder(gone)
		connectedRoots := blocksByRootRecorder(connected)

		r, rootA := pendingAttService(t, p1)
		rootB := [32]byte{'B'}
		r.savePendingAtt(gone.PeerID(), pendingAttFor(rootB, 1))
		r.savePendingAtt(connected.PeerID(), pendingAttFor(rootB, 2))
		require.NoError(t, p1.BHost.Network().ClosePeer(gone.PeerID()))

		require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))
		select {
		case got := <-connectedRoots:
			require.Equal(t, rootB, got)
		case <-time.After(5 * time.Second):
			t.Fatal("block was not requested from the connected sender")
		}
		select {
		case got := <-goneRoots:
			t.Fatalf("block %#x was requested from a disconnected peer", got)
		case <-time.After(200 * time.Millisecond):
		}

		r.pendingAttsLock.RLock()
		defer r.pendingAttsLock.RUnlock()
		require.Equal(t, false, r.pendingAttSenders[rootB][gone.PeerID()])
	})
}

func TestProcessPendingAtts_DownscoresSenderNotServingBlock(t *testing.T) {
	setup := func(t *testing.T) (*Service, *p2ptest.TestP2P, *p2ptest.TestP2P, [32]byte) {
		p1 := p2ptest.NewTestP2P(t)
		sender := p2ptest.NewTestP2P(t)
		p1.Connect(sender)
		r, rootA := pendingAttService(t, p1)
		return r, p1, sender, rootA
	}
	waitRequestDone := func(t *testing.T, r *Service) {
		require.Eventually(t, func() bool {
			r.pendingAttsLock.RLock()
			defer r.pendingAttsLock.RUnlock()
			return len(r.pendingAttRequestPeers) == 0
		}, 5*time.Second, 10*time.Millisecond, "in-flight request was not released")
	}

	t.Run("block not served", func(t *testing.T) {
		r, p1, sender, rootA := setup(t)
		senderRoots := blocksByRootRecorder(sender)
		r.savePendingAtt(sender.PeerID(), pendingAttFor([32]byte{'B'}, 1))
		r.savePendingAtt(sender.PeerID(), pendingAttFor([32]byte{'C'}, 1))

		require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))
		<-senderRoots
		waitRequestDone(t, r)

		// One request with missing blocks counts as a single bad response.
		count, err := p1.Peers().Scorers().BadResponsesScorer().Count(sender.PeerID())
		require.NoError(t, err)
		require.Equal(t, 1, count)

		// The sender is no longer asked for the blocks it did not serve.
		r.pendingAttsLock.RLock()
		defer r.pendingAttsLock.RUnlock()
		require.Equal(t, 0, len(r.pendingAttSenders))
	})

	t.Run("other sender asked next", func(t *testing.T) {
		r, _, failing, rootA := setup(t)
		other := p2ptest.NewTestP2P(t)
		r.cfg.p2p.(*p2ptest.TestP2P).Connect(other)
		failingRoots := blocksByRootRecorder(failing)
		otherRoots := blocksByRootRecorder(other)
		rootB := [32]byte{'B'}

		r.savePendingAtt(failing.PeerID(), pendingAttFor(rootB, 1))
		require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))
		<-failingRoots
		waitRequestDone(t, r)

		r.savePendingAtt(other.PeerID(), pendingAttFor(rootB, 2))
		require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))
		select {
		case got := <-otherRoots:
			require.Equal(t, rootB, got)
		case <-time.After(5 * time.Second):
			t.Fatal("block was not requested from the other sender")
		}
		select {
		case got := <-failingRoots:
			t.Fatalf("block %#x was requested again from the sender that did not serve it", got)
		case <-time.After(200 * time.Millisecond):
		}
	})

	t.Run("block obtained", func(t *testing.T) {
		r, p1, sender, rootA := setup(t)
		rootB := [32]byte{'B'}
		r.savePendingAtt(sender.PeerID(), pendingAttFor(rootB, 1))
		// Stand in for the sender serving the block: it lands in the pending block queue before the stream closes.
		for _, topic := range []string{p2p.RPCBlocksByRootTopicV1, p2p.RPCBlocksByRootTopicV2} {
			sender.BHost.SetStreamHandler(protocol.ID(topic+sender.Encoding().ProtocolSuffix()), func(stream network.Stream) {
				r.pendingQueueLock.Lock()
				r.seenPendingBlocks[rootB] = true
				r.pendingQueueLock.Unlock()
				_ = stream.Close()
			})
		}

		require.NoError(t, r.processPendingAttsForBlock(r.ctx, rootA))
		require.Eventually(t, func() bool { return r.isBlockInQueue(rootB) }, 5*time.Second, 10*time.Millisecond)
		waitRequestDone(t, r)

		count, err := p1.Peers().Scorers().BadResponsesScorer().Count(sender.PeerID())
		if err == nil {
			require.Equal(t, 0, count)
		}

		r.pendingAttsLock.RLock()
		defer r.pendingAttsLock.RUnlock()
		require.DeepEqual(t, map[peer.ID]bool{sender.PeerID(): true}, r.pendingAttSenders[rootB])
	})
}

func TestProcessPendingAtts_HasBlockSaveUnaggregatedAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)

	beaconState, privKeys := util.DeterministicGenesisState(t, validators)

	sb := util.NewBeaconBlock()
	util.SaveBlock(t, t.Context(), db, sb)
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	aggBits := bitfield.NewBitlist(8)
	aggBits.SetBitAt(1, true)
	att := &ethpb.Attestation{
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &ethpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &ethpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(t.Context(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att, committee)
	require.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	for _, i := range attestingIndices {
		att.Signature = privKeys[i].Sign(hashTreeRoot[:]).Marshal()
	}

	require.NoError(t, beaconState.SetGenesisTime(time.Now()))

	chain := &mock.ChainService{Genesis: time.Now(),
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Root:  att.Data.BeaconBlockRoot,
			Epoch: 0,
		},
	}

	done := make(chan *feed.Event, 1)
	defer close(done)
	opn := mock.NewEventFeedWrapper()
	sub := opn.Subscribe(done)
	defer sub.Unsubscribe()
	ctx, cancel := context.WithCancel(t.Context())
	r := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:                 p1,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:             attestations.NewPool(),
			attestationNotifier: &mock.SimpleNotifier{Feed: opn},
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(t.Context(), s, root))

	require.NoError(t, r.cfg.beaconDB.SaveState(t.Context(), s, root))

	r.blkRootToPendingAtts[root] = []any{att}
	require.NoError(t, r.processPendingAttsForBlock(t.Context(), root))

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case received := <-done:
				// make sure a single att was sent
				require.Equal(t, operation.UnaggregatedAttReceived, int(received.Type))
				return
			case <-ctx.Done():
				return
			}
		}
	})
	atts := r.cfg.attPool.UnaggregatedAttestations()
	assert.Equal(t, 1, len(atts), "Did not save unaggregated att")
	assert.DeepEqual(t, att, atts[0], "Incorrect saved att")
	assert.Equal(t, 0, len(r.cfg.attPool.AggregatedAttestations()), "Did save aggregated att")
	require.LogsContain(t, hook, "Verified and saved pending attestations to pool")
	wg.Wait()
	cancel()
}

func TestProcessPendingAtts_HasBlockSaveUnaggregatedAttElectra(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)

	beaconState, privKeys := util.DeterministicGenesisStateElectra(t, validators)

	sb := util.NewBeaconBlockElectra()
	util.SaveBlock(t, t.Context(), db, sb)
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	att := &ethpb.SingleAttestation{
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &ethpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &ethpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
	}

	committee, err := helpers.BeaconCommitteeFromState(t.Context(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	att.AttesterIndex = committee[0]
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	att.Signature = privKeys[committee[0]].Sign(hashTreeRoot[:]).Marshal()

	require.NoError(t, beaconState.SetGenesisTime(time.Now()))

	chain := &mock.ChainService{Genesis: time.Now(),
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Root:  att.Data.BeaconBlockRoot,
			Epoch: 0,
		},
	}
	done := make(chan *feed.Event, 1)
	defer close(done)
	opn := mock.NewEventFeedWrapper()
	sub := opn.Subscribe(done)
	defer sub.Unsubscribe()
	ctx, cancel := context.WithCancel(t.Context())
	r := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:                 p1,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:             attestations.NewPool(),
			attestationNotifier: &mock.SimpleNotifier{Feed: opn},
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()

	s, err := util.NewBeaconStateElectra()
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(t.Context(), s, root))

	require.NoError(t, r.cfg.beaconDB.SaveState(t.Context(), s, root))

	r.blkRootToPendingAtts[root] = []any{att}
	require.NoError(t, r.processPendingAttsForBlock(t.Context(), root))
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case received := <-done:
				// make sure a single att was sent
				require.Equal(t, operation.SingleAttReceived, int(received.Type))
				return
			case <-ctx.Done():
				return
			}
		}
	})
	atts := r.cfg.attPool.UnaggregatedAttestations()
	require.Equal(t, 1, len(atts), "Did not save unaggregated att")
	assert.DeepEqual(t, att.ToAttestationElectra(committee), atts[0], "Incorrect saved att")
	assert.Equal(t, 0, len(r.cfg.attPool.AggregatedAttestations()), "Did save aggregated att")
	require.LogsContain(t, hook, "Verified and saved pending attestations to pool")
	wg.Wait()
	cancel()
}

func TestProcessPendingAtts_HasBlockSaveUnAggregatedAttElectra_VerifyAlreadySeen(t *testing.T) {
	// Setup configuration and fork version schedule.
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()

	// Initialize logging, database, and P2P components.
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)
	currentSlot := 1 + (primitives.Slot(params.BeaconConfig().ElectraForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
	genesisOffset := time.Duration(currentSlot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	clock := startup.NewClock(time.Now().Add(-1*genesisOffset), params.BeaconConfig().GenesisValidatorsRoot)

	// Create genesis state and associated keys.
	beaconState, privKeys := util.DeterministicGenesisStateElectra(t, validators)
	require.NoError(t, beaconState.SetSlot(clock.CurrentSlot()))

	sb := util.NewBeaconBlockElectra()
	sb.Block.Slot = clock.CurrentSlot()
	util.SaveBlock(t, t.Context(), db, sb)

	// Save state with block root.
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	// Build a new attestation and its aggregate proof.
	att := &ethpb.SingleAttestation{
		CommitteeId: 8, // choose a non 0
		Data: &ethpb.AttestationData{
			Slot:            clock.CurrentSlot(),
			BeaconBlockRoot: root[:],
			Source:          &ethpb.Checkpoint{Epoch: clock.CurrentEpoch() - 1, Root: make([]byte, fieldparams.RootLength)},
			Target:          &ethpb.Checkpoint{Epoch: clock.CurrentEpoch(), Root: root[:]},
			CommitteeIndex:  0,
		},
	}

	// Retrieve the beacon committee and set the attester index.
	committee, err := helpers.BeaconCommitteeFromState(t.Context(), beaconState, att.Data.Slot, att.CommitteeId)
	assert.NoError(t, err)
	att.AttesterIndex = committee[0]

	// Compute attester domain and signature.
	attesterDomain, err := signing.Domain(beaconState.Fork(), clock.CurrentEpoch(), params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	att.SetSignature(privKeys[committee[0]].Sign(hashTreeRoot[:]).Marshal())

	// Set the genesis time.
	require.NoError(t, beaconState.SetGenesisTime(time.Now()))

	// Setup the chain service mock.
	chain := &mock.ChainService{
		Genesis: time.Now(),
		State:   beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Root:  att.Data.BeaconBlockRoot,
			Epoch: clock.CurrentEpoch() - 2,
		},
	}

	// Setup event feed and subscription.
	done := make(chan *feed.Event, 1)
	defer close(done)
	opn := mock.NewEventFeedWrapper()
	sub := opn.Subscribe(done)
	defer sub.Unsubscribe()

	// Create context and service configuration.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         &mockSync.Sync{IsSyncing: false},
			p2p:                 p1,
			beaconDB:            db,
			chain:               chain,
			clock:               clock,
			attPool:             attestations.NewPool(),
			attestationNotifier: &mock.SimpleNotifier{Feed: opn},
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()

	// Save a new beacon state and link it with the block root.
	slotOpt := func(s *ethpb.BeaconStateElectra) error { s.Slot = clock.CurrentSlot(); return nil }
	s, err := util.NewBeaconStateElectra(slotOpt)
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(t.Context(), s, root))

	// Add the pending attestation.
	r.blkRootToPendingAtts[root] = []any{
		att,
	}
	require.NoError(t, r.processPendingAttsForBlock(t.Context(), root))

	// Verify that the event feed receives the expected attestation.
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case received := <-done:
				// Ensure a single attestation event was sent.
				require.Equal(t, operation.SingleAttReceived, int(received.Type))
				return
			case <-ctx.Done():
				return
			}
		}
	})

	// Verify unaggregated attestations are saved correctly.
	atts := r.cfg.attPool.UnaggregatedAttestations()
	require.Equal(t, 1, len(atts), "Did not save unaggregated att")
	assert.DeepEqual(t, att.ToAttestationElectra(committee), atts[0], "Incorrect saved att")
	assert.Equal(t, 0, len(r.cfg.attPool.AggregatedAttestations()), "Did save aggregated att")
	require.LogsContain(t, hook, "Verified and saved pending attestations to pool")

	// Encode the attestation for pubsub and decode the message.
	buf := new(bytes.Buffer)
	_, err = p1.Encoding().EncodeGossip(buf, att)
	require.NoError(t, err)
	digest := r.currentForkDigest()
	topic := fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	_, err = r.decodePubsubMessage(m)
	require.NoError(t, err)

	// Validate the pubsub message and ignore it as it should already been seen.
	res, err := r.validateCommitteeIndexBeaconAttestation(ctx, "", m)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, res)

	// Wait for the event to complete.
	wg.Wait()
	cancel()
}

func TestProcessPendingAtts_NoBroadcastWithBadSignature(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	db := dbtest.SetupDB(t)
	p2p := p2ptest.NewTestP2P(t)
	st, privKeys := util.DeterministicGenesisState(t, 256)
	require.NoError(t, st.SetGenesisTime(time.Now()))
	b := util.NewBeaconBlock()
	r32, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, t.Context(), db, b)
	require.NoError(t, db.SaveState(t.Context(), st, r32))

	chain := &mock.ChainService{
		State:   st,
		Genesis: prysmTime.Now(),
		DB:      db,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Root:  r32[:],
			Epoch: 0,
		},
	}

	s := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:      p2p,
			beaconDB: db,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:  attestations.NewPool(),
		},
		blkRootToPendingAtts:           make(map[[32]byte][]any),
		signatureChan:                  make(chan *signatureVerifier, verifierLimit),
		seenAggregatedAttestationCache: lruwrpr.New(10),
	}
	go s.verifierRoutine()

	committee, err := helpers.BeaconCommitteeFromState(t.Context(), st, 0, 0)
	assert.NoError(t, err)
	// Arbitrary aggregator index for testing purposes.
	aggregatorIndex := committee[0]

	priv, err := bls.RandKey()
	require.NoError(t, err)
	aggBits := bitfield.NewBitlist(8)
	aggBits.SetBitAt(1, true)

	a := &ethpb.AggregateAttestationAndProof{
		Aggregate: &ethpb.Attestation{
			Signature:       priv.Sign([]byte("foo")).Marshal(),
			AggregationBits: aggBits,
			Data:            util.HydrateAttestationData(&ethpb.AttestationData{}),
		},
		AggregatorIndex: aggregatorIndex,
		SelectionProof:  make([]byte, fieldparams.BLSSignatureLength),
	}

	s.blkRootToPendingAtts[r32] = []any{&ethpb.SignedAggregateAttestationAndProof{Message: a, Signature: make([]byte, fieldparams.BLSSignatureLength)}}
	require.NoError(t, s.processPendingAttsForBlock(t.Context(), r32))

	assert.Equal(t, false, p2p.BroadcastCalled.Load(), "Broadcasted bad aggregate")

	// Clear pool.
	err = s.cfg.attPool.DeleteUnaggregatedAttestation(a.Aggregate)
	require.NoError(t, err)

	att := &ethpb.Attestation{
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: r32[:],
			Source:          &ethpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &ethpb.Checkpoint{Epoch: 0, Root: r32[:]},
		},
		AggregationBits: aggBits,
	}

	attestingIndices, err := attestation.AttestingIndices(att, committee)
	require.NoError(t, err)
	attesterDomain, err := signing.Domain(st.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	for _, i := range attestingIndices {
		att.Signature = privKeys[i].Sign(hashTreeRoot[:]).Marshal()
	}

	sszSlot := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(st, 0, &sszSlot, params.BeaconConfig().DomainSelectionProof, privKeys[aggregatorIndex])
	require.NoError(t, err)
	aggregateAndProof := &ethpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: aggregatorIndex,
	}
	aggreSig, err := signing.ComputeDomainAndSign(st, 0, aggregateAndProof, params.BeaconConfig().DomainAggregateAndProof, privKeys[aggregatorIndex])
	require.NoError(t, err)

	s.blkRootToPendingAtts[r32] = []any{&ethpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof, Signature: aggreSig}}
	require.NoError(t, s.processPendingAttsForBlock(t.Context(), r32))

	assert.Equal(t, true, p2p.BroadcastCalled.Load(), "The good aggregate was not broadcasted")

	cancel()
}

func TestProcessPendingAtts_HasBlockSaveAggregatedAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)

	beaconState, privKeys := util.DeterministicGenesisState(t, validators)

	sb := util.NewBeaconBlock()
	util.SaveBlock(t, t.Context(), db, sb)
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	aggBits := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	aggBits.SetBitAt(0, true)
	aggBits.SetBitAt(1, true)
	att := &ethpb.Attestation{
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &ethpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &ethpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(t.Context(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att, committee)
	require.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	sigs := make([]bls.Signature, len(attestingIndices))
	for i, indice := range attestingIndices {
		sig := privKeys[indice].Sign(hashTreeRoot[:])
		sigs[i] = sig
	}
	att.Signature = bls.AggregateSignatures(sigs).Marshal()

	// Arbitrary aggregator index for testing purposes.
	aggregatorIndex := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[aggregatorIndex])
	require.NoError(t, err)
	aggregateAndProof := &ethpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: aggregatorIndex,
	}
	aggreSig, err := signing.ComputeDomainAndSign(beaconState, 0, aggregateAndProof, params.BeaconConfig().DomainAggregateAndProof, privKeys[aggregatorIndex])
	require.NoError(t, err)

	require.NoError(t, beaconState.SetGenesisTime(time.Now()))

	chain := &mock.ChainService{Genesis: time.Now(),
		DB:    db,
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Root:  aggregateAndProof.Aggregate.Data.BeaconBlockRoot,
			Epoch: 0,
		}}
	ctx, cancel := context.WithCancel(t.Context())
	r := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:      p1,
			beaconDB: db,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:  attestations.NewPool(),
		},
		blkRootToPendingAtts:           make(map[[32]byte][]any),
		seenAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                  make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(t.Context(), s, root))

	require.NoError(t, r.cfg.beaconDB.SaveState(t.Context(), s, root))

	r.blkRootToPendingAtts[root] = []any{&ethpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof, Signature: aggreSig}}
	require.NoError(t, r.processPendingAttsForBlock(t.Context(), root))

	assert.Equal(t, 1, len(r.cfg.attPool.AggregatedAttestations()), "Did not save aggregated att")
	assert.DeepEqual(t, att, r.cfg.attPool.AggregatedAttestations()[0], "Incorrect saved att")
	atts := r.cfg.attPool.UnaggregatedAttestations()
	assert.Equal(t, 0, len(atts), "Did save aggregated att")
	require.LogsContain(t, hook, "Verified and saved pending attestations to pool")
	cancel()
}

func TestProcessPendingAtts_HasBlockSaveAggregatedAttElectra(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)

	beaconState, privKeys := util.DeterministicGenesisState(t, validators)

	sb := util.NewBeaconBlock()
	util.SaveBlock(t, t.Context(), db, sb)
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	committeeBits := primitives.NewAttestationCommitteeBits()
	committeeBits.SetBitAt(0, true)
	aggBits := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	aggBits.SetBitAt(0, true)
	aggBits.SetBitAt(1, true)
	att := &ethpb.AttestationElectra{
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &ethpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &ethpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
		CommitteeBits:   committeeBits,
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(t.Context(), beaconState, att.Data.Slot, att.GetCommitteeIndex())
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att, committee)
	require.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	sigs := make([]bls.Signature, len(attestingIndices))
	for i, indice := range attestingIndices {
		sig := privKeys[indice].Sign(hashTreeRoot[:])
		sigs[i] = sig
	}
	att.Signature = bls.AggregateSignatures(sigs).Marshal()

	// Arbitrary aggregator index for testing purposes.
	aggregatorIndex := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[aggregatorIndex])
	require.NoError(t, err)
	aggregateAndProof := &ethpb.AggregateAttestationAndProofElectra{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: aggregatorIndex,
	}
	aggreSig, err := signing.ComputeDomainAndSign(beaconState, 0, aggregateAndProof, params.BeaconConfig().DomainAggregateAndProof, privKeys[aggregatorIndex])
	require.NoError(t, err)

	require.NoError(t, beaconState.SetGenesisTime(time.Now()))

	chain := &mock.ChainService{Genesis: time.Now(),
		DB:    db,
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Root:  aggregateAndProof.Aggregate.Data.BeaconBlockRoot,
			Epoch: 0,
		}}
	ctx, cancel := context.WithCancel(t.Context())
	r := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:      p1,
			beaconDB: db,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:  attestations.NewPool(),
		},
		blkRootToPendingAtts:           make(map[[32]byte][]any),
		seenAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                  make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(t.Context(), s, root))

	r.blkRootToPendingAtts[root] = []any{&ethpb.SignedAggregateAttestationAndProofElectra{Message: aggregateAndProof, Signature: aggreSig}}
	require.NoError(t, r.processPendingAttsForBlock(t.Context(), root))

	assert.Equal(t, 1, len(r.cfg.attPool.AggregatedAttestations()), "Did not save aggregated att")
	assert.DeepEqual(t, att, r.cfg.attPool.AggregatedAttestations()[0], "Incorrect saved att")
	atts := r.cfg.attPool.UnaggregatedAttestations()
	assert.Equal(t, 0, len(atts), "Did save aggregated att")
	require.LogsContain(t, hook, "Verified and saved pending attestations to pool")
	cancel()
}

func TestProcessPendingAtts_BlockNotInForkChoice(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)

	beaconState, privKeys := util.DeterministicGenesisState(t, validators)

	sb := util.NewBeaconBlock()
	util.SaveBlock(t, t.Context(), db, sb)
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	aggBits := bitfield.NewBitlist(8)
	aggBits.SetBitAt(1, true)
	att := &ethpb.Attestation{
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &ethpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &ethpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(t.Context(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att, committee)
	require.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	for _, i := range attestingIndices {
		att.Signature = privKeys[i].Sign(hashTreeRoot[:]).Marshal()
	}

	aggregateAndProof := &ethpb.AggregateAttestationAndProof{
		Aggregate: att,
	}

	require.NoError(t, beaconState.SetGenesisTime(time.Now()))

	// Mock chain service that returns false for InForkchoice
	chain := &mock.ChainService{Genesis: time.Now(),
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Root:  aggregateAndProof.Aggregate.Data.BeaconBlockRoot,
			Epoch: 0,
		},
		// Set NotFinalized to true so InForkchoice returns false
		NotFinalized: true,
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:      p1,
			beaconDB: db,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:  attestations.NewPool(),
		},
		blkRootToPendingAtts: make(map[[32]byte][]any),
	}

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(t.Context(), s, root))

	// Add pending attestation
	r.blkRootToPendingAtts[root] = []any{&ethpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof}}

	// Process pending attestations - should return error because block is not in fork choice
	require.ErrorContains(t, "could not process unknown block root", r.processPendingAttsForBlock(t.Context(), root))

	// Verify attestations were not processed (should still be pending)
	assert.Equal(t, 1, len(r.blkRootToPendingAtts[root]), "Attestations should still be pending")
	assert.Equal(t, 0, len(r.cfg.attPool.UnaggregatedAttestations()), "Should not save attestation when block not in fork choice")
	assert.Equal(t, 0, len(r.cfg.attPool.AggregatedAttestations()), "Should not save attestation when block not in fork choice")
	require.LogsDoNotContain(t, hook, "Verified and saved pending attestations to pool")
}

func TestValidatePendingAtts_CanPruneOldAtts(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]any),
		pendingAttSenders:    make(map[[32]byte]map[peer.ID]bool),
	}

	// 100 Attestations per block root.
	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}
	r3 := [32]byte{'C'}

	for i := range primitives.Slot(100) {
		s.savePendingAtt("p", &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: i, BeaconBlockRoot: r1[:]}})
		s.savePendingAtt("p", &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: i, BeaconBlockRoot: r2[:]}})
		s.savePendingAtt("p", &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: i, BeaconBlockRoot: r3[:]}})
	}

	assert.Equal(t, 100, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 100, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")
	assert.Equal(t, 100, len(s.blkRootToPendingAtts[r3]), "Did not save pending atts")

	// Set current slot to 50, it should prune 19 attestations. (50 - 31)
	s.validatePendingAtts(t.Context(), 50)
	assert.Equal(t, 81, len(s.blkRootToPendingAtts[r1]), "Did not delete pending atts")
	assert.Equal(t, 81, len(s.blkRootToPendingAtts[r2]), "Did not delete pending atts")
	assert.Equal(t, 81, len(s.blkRootToPendingAtts[r3]), "Did not delete pending atts")

	// Set current slot to 100 + slot_duration, it should prune all the attestations.
	s.validatePendingAtts(t.Context(), 100+params.BeaconConfig().SlotsPerEpoch)
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r1]), "Did not delete pending atts")
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r2]), "Did not delete pending atts")
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r3]), "Did not delete pending atts")

	// Verify the keys are deleted.
	assert.Equal(t, 0, len(s.blkRootToPendingAtts), "Did not delete block keys")
}

func TestValidatePendingAtts_NoDuplicatingAtts(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]any),
		pendingAttSenders:    make(map[[32]byte]map[peer.ID]bool),
	}

	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}
	s.savePendingAtt("p", &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, BeaconBlockRoot: r1[:]}})
	s.savePendingAtt("p", &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2, BeaconBlockRoot: r2[:]}})
	s.savePendingAtt("p", &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2, BeaconBlockRoot: r2[:]}})

	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")
}

func TestSavePendingAtts_BeyondLimit(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]any),
		pendingAttSenders:    make(map[[32]byte]map[peer.ID]bool),
	}

	for i := range pendingAttsLimit {
		s.savePendingAtt("p", &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, BeaconBlockRoot: bytesutil.Bytes32(uint64(i))}})
	}
	r1 := [32]byte(bytesutil.Bytes32(0))
	r2 := [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit) - 1))

	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")

	for i := pendingAttsLimit; i < pendingAttsLimit+20; i++ {
		s.savePendingAtt("p", &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, BeaconBlockRoot: bytesutil.Bytes32(uint64(i))}})
	}

	r1 = [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit)))
	r2 = [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit) + 10))

	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r1]), "Saved pending atts")
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r2]), "Saved pending atts")
}

func Test_pendingAggregatesAreEqual(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		a := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           1,
						CommitteeIndex: 1,
					},
					AggregationBits: bitfield.Bitlist{0b1111},
				}}}
		b := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           1,
						CommitteeIndex: 1,
					},
					AggregationBits: bitfield.Bitlist{0b1111},
				}}}
		assert.Equal(t, true, pendingAggregatesAreEqual(a, b, includeAggregatorIndex))
	})
	t.Run("different version", func(t *testing.T) {
		a := &ethpb.SignedAggregateAttestationAndProof{Message: &ethpb.AggregateAttestationAndProof{AggregatorIndex: 1}}
		b := &ethpb.SignedAggregateAttestationAndProofElectra{Message: &ethpb.AggregateAttestationAndProofElectra{AggregatorIndex: 1}}
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b, includeAggregatorIndex))
	})
	t.Run("different aggregator index", func(t *testing.T) {
		a := &ethpb.SignedAggregateAttestationAndProof{Message: &ethpb.AggregateAttestationAndProof{AggregatorIndex: 1}}
		b := &ethpb.SignedAggregateAttestationAndProof{Message: &ethpb.AggregateAttestationAndProof{AggregatorIndex: 2}}
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b, includeAggregatorIndex))
	})
	t.Run("different slot", func(t *testing.T) {
		a := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           1,
						CommitteeIndex: 1,
					},
					AggregationBits: bitfield.Bitlist{0b1111},
				}}}
		b := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           2,
						CommitteeIndex: 1,
					},
					AggregationBits: bitfield.Bitlist{0b1111},
				}}}
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b, includeAggregatorIndex))
	})
	t.Run("different committee index", func(t *testing.T) {
		a := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           1,
						CommitteeIndex: 1,
					},
					AggregationBits: bitfield.Bitlist{0b1111},
				}}}
		b := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           1,
						CommitteeIndex: 2,
					},
					AggregationBits: bitfield.Bitlist{0b1111},
				}}}
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b, includeAggregatorIndex))
	})
	t.Run("different aggregation bits", func(t *testing.T) {
		a := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           1,
						CommitteeIndex: 1,
					},
					AggregationBits: bitfield.Bitlist{0b1111},
				}}}
		b := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           1,
						CommitteeIndex: 1,
					},
					AggregationBits: bitfield.Bitlist{0b1000},
				}}}
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b, includeAggregatorIndex))
	})
	t.Run("different aggregator index should be equal while ignoring aggregator index", func(t *testing.T) {
		a := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				AggregatorIndex: 1,
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           1,
						CommitteeIndex: 1,
					},
					AggregationBits: bitfield.Bitlist{0b1111},
				}}}
		b := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				AggregatorIndex: 2,
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:           1,
						CommitteeIndex: 1,
					},
					AggregationBits: bitfield.Bitlist{0b1111},
				}}}
		assert.Equal(t, true, pendingAggregatesAreEqual(a, b, ignoreAggregatorIndex))
	})
}

func Test_pendingAttsAreEqual(t *testing.T) {
	t.Run("equal Phase0", func(t *testing.T) {
		a := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1111}}
		b := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1111}}
		assert.Equal(t, true, pendingAttsAreEqual(a, b))
	})
	t.Run("equal Electra", func(t *testing.T) {
		a := &ethpb.SingleAttestation{Data: &ethpb.AttestationData{Slot: 1}, AttesterIndex: 1}
		b := &ethpb.SingleAttestation{Data: &ethpb.AttestationData{Slot: 1}, AttesterIndex: 1}
		assert.Equal(t, true, pendingAttsAreEqual(a, b))
	})
	t.Run("different version", func(t *testing.T) {
		a := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1111}}
		b := &ethpb.SingleAttestation{Data: &ethpb.AttestationData{Slot: 1}, AttesterIndex: 1}
		assert.Equal(t, false, pendingAttsAreEqual(a, b))
	})
	t.Run("different slot", func(t *testing.T) {
		a := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1111}}
		b := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1111}}
		assert.Equal(t, false, pendingAttsAreEqual(a, b))
	})
	t.Run("different committee index", func(t *testing.T) {
		a := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1111}}
		b := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 2}, AggregationBits: bitfield.Bitlist{0b1111}}
		assert.Equal(t, false, pendingAttsAreEqual(a, b))
	})
	t.Run("different aggregation bits", func(t *testing.T) {
		a := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1111}}
		b := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1000}}
		assert.Equal(t, false, pendingAttsAreEqual(a, b))
	})
	t.Run("different attester index", func(t *testing.T) {
		a := &ethpb.SingleAttestation{Data: &ethpb.AttestationData{Slot: 1}, AttesterIndex: 1}
		b := &ethpb.SingleAttestation{Data: &ethpb.AttestationData{Slot: 1}, AttesterIndex: 2}
		assert.Equal(t, false, pendingAttsAreEqual(a, b))
	})
}

func TestSavePendingAtts_RecordsSenders(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]any),
		pendingAttSenders:    make(map[[32]byte]map[peer.ID]bool),
	}
	root := [32]byte{'A'}
	want := make(map[peer.ID]bool)
	for i := range 10 {
		pid := peer.ID(fmt.Sprintf("peer-%d", i))
		s.savePendingAtt(pid, pendingAttFor(root, primitives.Slot(i)))
		// A second attestation from the same peer does not change anything.
		s.savePendingAtt(pid, pendingAttFor(root, primitives.Slot(100+i)))
		want[pid] = true
	}
	require.DeepEqual(t, want, s.pendingAttSenders[root])

	// Pruning the attestations drops the senders with them.
	s.validatePendingAtts(t.Context(), 200+params.BeaconConfig().SlotsPerEpoch)
	require.Equal(t, 0, len(s.pendingAttSenders))
}
