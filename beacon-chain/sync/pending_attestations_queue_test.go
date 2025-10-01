package sync

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/ethereum/go-ethereum/p2p/enr"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/prysmaticlabs/go-bitfield"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

var verifierLimit = 1000

func TestProcessPendingAtts_NoBlockRequestBlock(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	assert.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")
	p1.Peers().Add(new(enr.Record), p2.PeerID(), nil, network.DirOutbound)
	p1.Peers().SetConnectionState(p2.PeerID(), peers.Connected)
	p1.Peers().SetChainState(p2.PeerID(), &ethpb.StatusV2{})

	// Create and save block 'A' to DB
	blockA := util.NewBeaconBlock()
	util.SaveBlock(t, t.Context(), db, blockA)
	rootA, err := blockA.Block.HashTreeRoot()
	require.NoError(t, err)

	// Save state for block 'A'
	stateA, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(t.Context(), stateA, rootA))

	// Setup chain service with block 'A' in forkchoice
	chain := &mock.ChainService{
		Genesis:             prysmTime.Now(),
		FinalizedCheckPoint: &ethpb.Checkpoint{},
		// NotFinalized: false means InForkchoice returns true
	}

	r := &Service{
		cfg:                  &config{p2p: p1, beaconDB: db, chain: chain, clock: startup.NewClock(chain.Genesis, chain.ValidatorsRoot)},
		blkRootToPendingAtts: make(map[[32]byte][]any),
		seenPendingBlocks:    make(map[[32]byte]bool),
		chainStarted:         abool.New(),
	}

	// Add pending attestations for OTHER block roots (not block A)
	// These are blocks we don't have yet, so they should be requested
	attB := &ethpb.Attestation{Data: &ethpb.AttestationData{
		BeaconBlockRoot: bytesutil.PadTo([]byte{'B'}, 32),
		Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
	}}
	attC := &ethpb.Attestation{Data: &ethpb.AttestationData{
		BeaconBlockRoot: bytesutil.PadTo([]byte{'C'}, 32),
		Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
	}}
	r.blkRootToPendingAtts[[32]byte{'B'}] = []any{attB}
	r.blkRootToPendingAtts[[32]byte{'C'}] = []any{attC}

	// Process block A (which exists and has no pending attestations)
	// This should skip processing attestations for A and request blocks B and C
	require.NoError(t, r.processPendingAttsForBlock(t.Context(), rootA))
	require.LogsContain(t, hook, "Requesting block by root")
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
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	}()
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
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	}()
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
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	}()

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
	digest, err := r.currentForkDigest()
	require.NoError(t, err)
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
	}

	// 100 Attestations per block root.
	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}
	r3 := [32]byte{'C'}

	for i := primitives.Slot(0); i < 100; i++ {
		s.savePendingAtt(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: i, BeaconBlockRoot: r1[:]}})
		s.savePendingAtt(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: i, BeaconBlockRoot: r2[:]}})
		s.savePendingAtt(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: i, BeaconBlockRoot: r3[:]}})
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
	}

	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}
	s.savePendingAtt(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, BeaconBlockRoot: r1[:]}})
	s.savePendingAtt(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2, BeaconBlockRoot: r2[:]}})
	s.savePendingAtt(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2, BeaconBlockRoot: r2[:]}})

	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")
}

func TestSavePendingAtts_BeyondLimit(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]any),
	}

	for i := 0; i < pendingAttsLimit; i++ {
		s.savePendingAtt(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, BeaconBlockRoot: bytesutil.Bytes32(uint64(i))}})
	}
	r1 := [32]byte(bytesutil.Bytes32(0))
	r2 := [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit) - 1))

	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")

	for i := pendingAttsLimit; i < pendingAttsLimit+20; i++ {
		s.savePendingAtt(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, BeaconBlockRoot: bytesutil.Bytes32(uint64(i))}})
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
		assert.Equal(t, true, pendingAggregatesAreEqual(a, b))
	})
	t.Run("different version", func(t *testing.T) {
		a := &ethpb.SignedAggregateAttestationAndProof{Message: &ethpb.AggregateAttestationAndProof{AggregatorIndex: 1}}
		b := &ethpb.SignedAggregateAttestationAndProofElectra{Message: &ethpb.AggregateAttestationAndProofElectra{AggregatorIndex: 1}}
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b))
	})
	t.Run("different aggregator index", func(t *testing.T) {
		a := &ethpb.SignedAggregateAttestationAndProof{Message: &ethpb.AggregateAttestationAndProof{AggregatorIndex: 1}}
		b := &ethpb.SignedAggregateAttestationAndProof{Message: &ethpb.AggregateAttestationAndProof{AggregatorIndex: 2}}
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b))
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
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b))
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
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b))
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
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b))
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
