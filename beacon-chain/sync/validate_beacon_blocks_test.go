package sync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	coreTime "github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	slashingsmock "github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings/mock"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	gcache "github.com/patrickmn/go-cache"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

// General note for writing validation tests: Use a random value for any field
// on the beacon block to avoid hitting shared global cache conditions across
// tests in this package.

func TestValidateBeaconBlockPubSub_InvalidSignature(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	badPrivKeyIdx := proposerIdx + 1 // We generate a valid signature from a wrong private key which fails to verify
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[badPrivKeyIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		DB: db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "invalid signature", err)
	result := res == pubsub.ValidationReject
	assert.Equal(t, true, result)
}

func TestValidateBeaconBlockPubSub_InvalidSignature_MarksBlockAsBad(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	badPrivKeyIdx := proposerIdx + 1 // We generate a valid signature from a wrong private key which fails to verify
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[badPrivKeyIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		DB: db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	blockRoot, err := msg.Block.HashTreeRoot()
	require.NoError(t, err)

	// Verify block is not marked as bad initially
	assert.Equal(t, false, r.hasBadBlock(blockRoot), "block should not be marked as bad initially")

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "invalid signature", err)
	result := res == pubsub.ValidationReject
	assert.Equal(t, true, result)

	// Verify block is now marked as bad after invalid signature
	assert.Equal(t, true, r.hasBadBlock(blockRoot), "block should be marked as bad after invalid signature")
}

func TestValidateBeaconBlockPubSub_BlockAlreadyPresentInDB(t *testing.T) {
	db := dbtest.SetupDB(t)
	ctx := t.Context()

	p := p2ptest.NewTestP2P(t)
	msg := util.NewBeaconBlock()
	msg.Block.Slot = 100
	msg.Block.ParentRoot = util.Random32Bytes(t)
	util.SaveBlock(t, t.Context(), db, msg)

	chainService := &mock.ChainService{Genesis: time.Now()}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block present in DB should be ignored")
}

func TestValidateBeaconBlockPubSub_CanRecoverStateSummary(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		DB: db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_IsInCache(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(t.Context(), copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		InitSyncBlockRoots: map[[32]byte]bool{bRoot: true},
		DB:                 db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_ValidProposerSignature(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		DB: db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_WithLookahead(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	// The next block is only 1 epoch ahead so as to not induce a new seed.
	blkSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(coreTime.NextEpoch(copied)))
	copied, err = transition.ProcessSlots(t.Context(), copied, blkSlot)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = blkSlot
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	offset := int64(blkSlot.Mul(params.BeaconConfig().SecondsPerSlot))
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-offset, 0),
		DB:    db,
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
		subHandler:          newSubTopicHandler(),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_AdvanceEpochsForState(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	// The next block is at least 2 epochs ahead to induce shuffling and a new seed.
	blkSlot := params.BeaconConfig().SlotsPerEpoch * 2
	copied, err = transition.ProcessSlots(t.Context(), copied, blkSlot)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = blkSlot
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	offset := int64(blkSlot.Mul(params.BeaconConfig().SecondsPerSlot))
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-offset, 0),
		DB:    db,
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_Syncing(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	b := []byte("sk")
	b32 := bytesutil.ToBytes32(b)
	sk, err := bls.SecretKeyFromBytes(b32[:])
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Signature = sk.Sign([]byte("data")).Marshal()
	chainService := &mock.ChainService{
		Genesis: time.Now(),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: true},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
		},
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block is ignored until fully synced")
}

func TestValidateBeaconBlockPubSub_IgnoreAndQueueBlocksFromNearFuture(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()

	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlock()
	msg.Block.Slot = 2 // two slots in future
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.ProposerIndex = proposerIdx
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Now(),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		State: beaconState}
	r := &Service{
		cfg: &config{
			p2p:           p,
			beaconDB:      db,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		chainStarted:        abool.New(),
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "early block, with current slot", err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "early block should be ignored and queued")

	// check if the block is inserted in the Queue
	assert.Equal(t, true, len(r.pendingBlocksInCache(msg.Block.Slot)) == 1)
}

func TestValidateBeaconBlockPubSub_RejectBlocksFromFuture(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	b := []byte("sk")
	b32 := bytesutil.ToBytes32(b)
	sk, err := bls.SecretKeyFromBytes(b32[:])
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.Slot = 10
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Signature = sk.Sign([]byte("data")).Marshal()

	chainService := &mock.ChainService{Genesis: time.Now()}
	r := &Service{
		cfg: &config{
			p2p:           p,
			beaconDB:      db,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
		},
		chainStarted:        abool.New(),
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block from the future should be ignored")
}

func TestValidateBeaconBlockPubSub_RejectBlocksFromThePast(t *testing.T) {
	db := dbtest.SetupDB(t)
	b := []byte("sk")
	b32 := bytesutil.ToBytes32(b)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	sk, err := bls.SecretKeyFromBytes(b32[:])
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Block.Slot = 10
	msg.Signature = sk.Sign([]byte("data")).Marshal()

	genesisTime := time.Now()
	chainService := &mock.ChainService{
		Genesis: time.Unix(genesisTime.Unix()-1000, 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 1,
		},
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "greater or equal to block slot", err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block from the past should be ignored")
}

func TestValidateBeaconBlockPubSub_SeenProposerSlot(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, beaconState)
	require.NoError(t, err)

	msg := util.NewBeaconBlock()
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	// Create a clone of the same block (same signature, not an equivocation)
	msgClone := util.NewBeaconBlock()
	msgClone.Block.Slot = 1
	msgClone.Block.ProposerIndex = proposerIdx
	msgClone.Block.ParentRoot = bRoot[:]
	msgClone.Signature = msg.Signature // Use the same signature

	signedBlock, err := blocks.NewSignedBeaconBlock(msg)
	require.NoError(t, err)

	slashingPool := &slashingsmock.PoolMock{}
	chainService := &mock.ChainService{
		Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State:   beaconState,
		Block:   signedBlock, // Set the first block as the head block
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			slashingPool:  slashingPool,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}

	// Mark the proposer/slot as seen
	r.setSeenBlockIndexSlot(msg.Block.Slot, msg.Block.ProposerIndex)
	time.Sleep(10 * time.Millisecond) // Wait for cached value to pass through buffers

	// Prepare and validate the second message (clone)
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msgClone)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msgClone)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	// Since this is not an equivocation (same signature), it should be ignored
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	assert.Equal(t, pubsub.ValidationIgnore, res, "block with same signature should be ignored")

	// Verify no slashings were created
	assert.Equal(t, 0, len(slashingPool.PendingPropSlashings), "Expected no slashings for same signature")
}

func TestValidateBeaconBlockPubSub_FilterByFinalizedEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	parent := util.NewBeaconBlock()
	util.SaveBlock(t, t.Context(), db, parent)
	parentRoot, err := parent.Block.HashTreeRoot()
	require.NoError(t, err)
	chain := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 1,
		},
		ValidatorsRoot: params.BeaconConfig().GenesisValidatorsRoot,
	}

	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			chain:         chain,
			clock:         startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			blockNotifier: chain.BlockNotifier(),
			attPool:       attestations.NewPool(),
			initialSync:   &mockSync.Sync{IsSyncing: false},
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	b := util.NewBeaconBlock()
	b.Block.Slot = 1
	b.Block.ParentRoot = parentRoot[:]
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, b)
	require.NoError(t, err)
	digest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, params.BeaconConfig().GenesisValidatorsRoot[:])
	assert.NoError(t, err)
	topic := fmt.Sprintf(p2p.GossipTypeMapping[reflect.TypeOf(b)], digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, err := r.validateBeaconBlockPubSub(t.Context(), "", m)
	_ = err
	assert.Equal(t, pubsub.ValidationIgnore, res)

	hook.Reset()
	b.Block.Slot = params.BeaconConfig().SlotsPerEpoch
	buf = new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, b)
	require.NoError(t, err)
	m = &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, err = r.validateBeaconBlockPubSub(t.Context(), "", m)
	assert.NoError(t, err)
	assert.Equal(t, pubsub.ValidationIgnore, res)
}

func TestValidateBeaconBlockPubSub_ParentNotFinalizedDescendant(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{
		Genesis:      time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		NotFinalized: true,
		State:        beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		VerifyBlkDescendantErr: errors.New("not part of finalized chain"),
		DB:                     db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.Equal(t, pubsub.ValidationReject, res, "Wrong validation result returned")
	require.ErrorContains(t, "not descendant of finalized checkpoint", err)
}

func TestValidateBeaconBlockPubSub_InvalidParentBlock(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = 1
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	// Mutate Signature
	copy(msg.Signature[:4], []byte{1, 2, 3, 4})
	currBlockRoot, err := msg.Block.HashTreeRoot()
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "invalid signature", err)
	assert.Equal(t, res, pubsub.ValidationReject, "block with invalid signature should be rejected")

	require.NoError(t, copied.SetSlot(2))
	proposerIdx, err = helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg = util.NewBeaconBlock()
	msg.Block.Slot = 2
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.ParentRoot = currBlockRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	buf = new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	m = &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	chainService = &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(2*params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
		}}
	r.cfg.chain = chainService
	r.cfg.clock = startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)

	res, err = r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "has an invalid parent", err)
	// Expect block with bad parent to fail too
	assert.Equal(t, res, pubsub.ValidationReject, "block with invalid parent should be ignored")
}

func TestValidateBeaconBlockPubSub_InsertValidPendingBlock(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlock()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = 1
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "unknown parent for block", err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block with unknown parent should be ignored")
	bRoot, err = msg.Block.HashTreeRoot()
	assert.NoError(t, err)
	assert.Equal(t, true, r.seenPendingBlocks[bRoot])
}

func TestValidateBeaconBlockPubSub_RejectBlocksFromBadParent(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()

	beaconState, privKeys := util.DeterministicGenesisState(t, 100)
	parentBlock := util.NewBeaconBlock()
	parentBlock.Block.ParentRoot = bytesutil.PadTo([]byte("foo"), 32)
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))

	copied := beaconState.Copy()
	// The next block is at least 2 epochs ahead to induce shuffling and a new seed.
	blkSlot := params.BeaconConfig().SlotsPerEpoch * 2
	copied, err = transition.ProcessSlots(t.Context(), copied, blkSlot)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlock()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = blkSlot

	perSlot := params.BeaconConfig().SecondsPerSlot
	// current slot time
	slotsSinceGenesis := primitives.Slot(1000)
	// max uint, divided by slot time. But avoid losing precision too much.
	overflowBase := (1 << 63) / (perSlot >> 1)
	msg.Block.Slot = slotsSinceGenesis.Add(overflowBase)

	// valid block
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	genesisTime := time.Now()

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{
		Genesis: time.Unix(genesisTime.Unix()-int64(slotsSinceGenesis.Mul(perSlot)), 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
		},
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	r.setBadBlock(ctx, bytesutil.ToBytes32(msg.Block.ParentRoot))

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.ErrorContains(t, "invalid parent", err)
	assert.Equal(t, res, pubsub.ValidationReject)
}

func TestService_setBadBlock_DoesntSetWithContextErr(t *testing.T) {
	s := Service{}
	s.initCaches()

	root := [32]byte{'b', 'a', 'd'}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.setBadBlock(ctx, root)
	if s.hasBadBlock(root) {
		t.Error("Set bad root with cancelled context")
	}
}

func TestService_isBlockQueueable(t *testing.T) {
	currentTime := time.Now().Round(time.Second)
	genesisTime := currentTime.Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	blockSlot := primitives.Slot(1)

	// slot time within MAXIMUM_GOSSIP_CLOCK_DISPARITY, so don't queue the block.
	receivedTime := currentTime.Add(-400 * time.Millisecond)
	result := isBlockQueueable(genesisTime, blockSlot, receivedTime)
	assert.Equal(t, false, result)

	// slot time just above MAXIMUM_GOSSIP_CLOCK_DISPARITY, so queue the block.
	receivedTime = currentTime.Add(-600 * time.Millisecond)
	result = isBlockQueueable(genesisTime, blockSlot, receivedTime)
	assert.Equal(t, true, result)
}

func TestValidateBeaconBlockPubSub_ValidExecutionPayload(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisStateBellatrix(t, 100)
	parentBlock := util.NewBeaconBlockBellatrix()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	now := time.Now()
	require.NoError(t, beaconState.SetGenesisTime(now))
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlockBellatrix()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Body.ExecutionPayload.Timestamp = uint64(now.Unix()) + params.BeaconConfig().SecondsPerSlot
	msg.Block.Body.ExecutionPayload.GasUsed = 10
	msg.Block.Body.ExecutionPayload.GasLimit = 11
	msg.Block.Body.ExecutionPayload.BlockHash = bytesutil.PadTo([]byte("blockHash"), 32)
	msg.Block.Body.ExecutionPayload.ParentHash = bytesutil.PadTo([]byte("parentHash"), 32)
	msg.Block.Body.ExecutionPayload.Transactions = append(msg.Block.Body.ExecutionPayload.Transactions, []byte("transaction 1"), []byte("transaction 2"))
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: now.Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
		ValidatorsRoot: params.BeaconConfig().GenesisValidatorsRoot,
		DB:             db,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	genesisValidatorsRoot := r.cfg.clock.GenesisValidatorsRoot()
	BellatrixDigest, err := signing.ComputeForkDigest(params.BeaconConfig().BellatrixForkVersion, genesisValidatorsRoot[:])
	require.NoError(t, err)
	topic = r.addDigestToTopic(topic, BellatrixDigest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.NoError(t, err)
	result := res == pubsub.ValidationAccept
	require.Equal(t, true, result)
}

func TestValidateBeaconBlockPubSub_InvalidPayloadTimestamp(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	beaconState, privKeys := util.DeterministicGenesisStateBellatrix(t, 100)
	parentBlock := util.NewBeaconBlockBellatrix()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	presentTime := time.Now().Unix()
	msg := util.NewBeaconBlockBellatrix()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Body.ExecutionPayload.Timestamp = uint64(presentTime - 600) // add an invalid timestamp
	msg.Block.Body.ExecutionPayload.GasUsed = 10
	msg.Block.Body.ExecutionPayload.GasLimit = 11
	msg.Block.Body.ExecutionPayload.BlockHash = bytesutil.PadTo([]byte("blockHash"), 32)
	msg.Block.Body.ExecutionPayload.ParentHash = bytesutil.PadTo([]byte("parentHash"), 32)
	msg.Block.Body.ExecutionPayload.Transactions = append(msg.Block.Body.ExecutionPayload.Transactions, []byte("transaction 1"), []byte("transaction 2"))
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(presentTime-int64(params.BeaconConfig().SecondsPerSlot), 0),
		DB: db,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	genesisValidatorsRoot := r.cfg.clock.GenesisValidatorsRoot()
	BellatrixDigest, err := signing.ComputeForkDigest(params.BeaconConfig().BellatrixForkVersion, genesisValidatorsRoot[:])
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, BellatrixDigest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.NotNil(t, err)
	result := res == pubsub.ValidationReject
	assert.Equal(t, true, result)
}

func Test_validateBellatrixBeaconBlock(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	stateGen := stategen.New(db, doublylinkedtree.New())
	presentTime := time.Now().Unix()
	chainService := &mock.ChainService{Genesis: time.Unix(presentTime-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	st, _ := util.DeterministicGenesisStateAltair(t, 1)
	b := util.NewBeaconBlockBellatrix()
	blk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.ErrorContains(t, "block and state are not the same version", r.validateBellatrixBeaconBlock(ctx, st, blk.Block()))
}

func Test_validateBellatrixBeaconBlockParentValidation(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	stateGen := stategen.New(db, doublylinkedtree.New())

	beaconState, privKeys := util.DeterministicGenesisStateBellatrix(t, 100)
	parentBlock := util.NewBeaconBlockBellatrix()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlockBellatrix()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Body.ExecutionPayload.Timestamp = uint64(beaconState.GenesisTime().Add(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second).Unix())
	msg.Block.Body.ExecutionPayload.GasUsed = 10
	msg.Block.Body.ExecutionPayload.GasLimit = 11
	msg.Block.Body.ExecutionPayload.BlockHash = bytesutil.PadTo([]byte("blockHash"), 32)
	msg.Block.Body.ExecutionPayload.ParentHash = bytesutil.PadTo([]byte("parentHash"), 32)
	msg.Block.Body.ExecutionPayload.Transactions = append(msg.Block.Body.ExecutionPayload.Transactions, []byte("transaction 1"), []byte("transaction 2"))
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	blk, err := blocks.NewSignedBeaconBlock(msg)
	require.NoError(t, err)

	chainService := &mock.ChainService{Genesis: beaconState.GenesisTime(),
		OptimisticRoots: make(map[[32]byte]bool),
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}

	chainService.OptimisticRoots[blk.Block().ParentRoot()] = true
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}
	require.ErrorContains(t, "parent of the block is optimistic", r.validateBellatrixBeaconBlock(ctx, beaconState, blk.Block()))
}

func Test_validateBeaconBlockProcessingWhenParentIsOptimistic(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	stateGen := stategen.New(db, doublylinkedtree.New())

	beaconState, privKeys := util.DeterministicGenesisStateBellatrix(t, 100)
	parentBlock := util.NewBeaconBlockBellatrix()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlockBellatrix()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Body.ExecutionPayload.Timestamp = uint64(beaconState.GenesisTime().Add(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second).Unix())
	msg.Block.Body.ExecutionPayload.GasUsed = 10
	msg.Block.Body.ExecutionPayload.GasLimit = 11
	msg.Block.Body.ExecutionPayload.BlockHash = bytesutil.PadTo([]byte("blockHash"), 32)
	msg.Block.Body.ExecutionPayload.ParentHash = bytesutil.PadTo([]byte("parentHash"), 32)
	msg.Block.Body.ExecutionPayload.Transactions = append(msg.Block.Body.ExecutionPayload.Transactions, []byte("transaction 1"), []byte("transaction 2"))
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	chainService := &mock.ChainService{Genesis: beaconState.GenesisTime(),
		ValidatorsRoot: params.BeaconConfig().GenesisValidatorsRoot,
		DB:             db,
		Optimistic:     true,
		FinalizedCheckPoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
	genesisValidatorsRoot := r.cfg.clock.GenesisValidatorsRoot()
	BellatrixDigest, err := signing.ComputeForkDigest(params.BeaconConfig().BellatrixForkVersion, genesisValidatorsRoot[:])
	require.NoError(t, err)
	topic = r.addDigestToTopic(topic, BellatrixDigest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
}

func Test_getBlockFields(t *testing.T) {
	hook := logTest.NewGlobal()

	// Nil
	log.WithFields(getBlockFields(nil)).Info("nil block")
	// Good block
	b := util.NewBeaconBlockBellatrix()
	wb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	log.WithFields(getBlockFields(wb)).Info("bad block")

	require.LogsContain(t, hook, "nil block")
	require.LogsContain(t, hook, "bad block")
}

func Test_validateDenebBeaconBlock(t *testing.T) {
	bb := util.NewBeaconBlockBellatrix()
	b, err := blocks.NewSignedBeaconBlock(bb)
	require.NoError(t, err)
	require.NoError(t, validateDenebBeaconBlock(b.Block()))

	bd := util.NewBeaconBlockDeneb()
	bd.Block.Body.BlobKzgCommitments = make([][]byte, 7)
	bdb, err := blocks.NewSignedBeaconBlock(bd)
	require.NoError(t, err)
	require.ErrorIs(t, validateDenebBeaconBlock(bdb.Block()), errRejectCommitmentLen)
}

func TestDetectAndBroadcastEquivocation(t *testing.T) {
	ctx := t.Context()
	p := p2ptest.NewTestP2P(t)
	beaconState, privKeys := util.DeterministicGenesisState(t, 100)

	t.Run("no equivocation", func(t *testing.T) {
		block := util.NewBeaconBlock()
		block.Block.Slot = 1
		block.Block.ProposerIndex = 0

		sig, err := signing.ComputeDomainAndSign(beaconState, 0, block.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		block.Signature = sig

		// Create head block with different slot/proposer
		headBlock := util.NewBeaconBlock()
		headBlock.Block.Slot = 2          // Different slot
		headBlock.Block.ProposerIndex = 1 // Different proposer
		signedHeadBlock, err := blocks.NewSignedBeaconBlock(headBlock)
		require.NoError(t, err)

		chainService := &mock.ChainService{
			State:   beaconState,
			Genesis: time.Now(),
			Block:   signedHeadBlock,
		}

		slashingPool := &slashingsmock.PoolMock{}
		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: slashingPool,
			},
			seenBlockCache: lruwrpr.New(10),
		}

		signedBlock, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)

		err = r.detectAndBroadcastEquivocation(ctx, signedBlock)
		require.NoError(t, err)
		assert.Equal(t, 0, len(slashingPool.PendingPropSlashings), "Expected no slashings")
	})

	t.Run("equivocation detected", func(t *testing.T) {
		// Create head block
		headBlock := util.NewBeaconBlock()
		headBlock.Block.Slot = 1
		headBlock.Block.ProposerIndex = 0
		headBlock.Block.ParentRoot = bytesutil.PadTo([]byte("parent1"), 32)
		sig1, err := signing.ComputeDomainAndSign(beaconState, 0, headBlock.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		headBlock.Signature = sig1

		// Create second block with same slot/proposer but different contents
		newBlock := util.NewBeaconBlock()
		newBlock.Block.Slot = 1
		newBlock.Block.ProposerIndex = 0
		newBlock.Block.ParentRoot = bytesutil.PadTo([]byte("parent2"), 32)
		sig2, err := signing.ComputeDomainAndSign(beaconState, 0, newBlock.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		newBlock.Signature = sig2

		signedHeadBlock, err := blocks.NewSignedBeaconBlock(headBlock)
		require.NoError(t, err)

		slashingPool := &slashingsmock.PoolMock{}
		chainService := &mock.ChainService{
			State:   beaconState,
			Genesis: time.Now(),
			Block:   signedHeadBlock,
		}

		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: slashingPool,
			},
			seenBlockCache: lruwrpr.New(10),
		}

		signedNewBlock, err := blocks.NewSignedBeaconBlock(newBlock)
		require.NoError(t, err)

		err = r.detectAndBroadcastEquivocation(ctx, signedNewBlock)
		require.NoError(t, err)

		// Verify slashing was inserted
		require.Equal(t, 1, len(slashingPool.PendingPropSlashings), "Expected a slashing to be inserted")
		slashing := slashingPool.PendingPropSlashings[0]
		assert.Equal(t, primitives.ValidatorIndex(0), slashing.Header_1.Header.ProposerIndex, "Wrong proposer index")
		assert.Equal(t, primitives.Slot(1), slashing.Header_1.Header.Slot, "Wrong slot")
	})

	t.Run("same signature", func(t *testing.T) {
		// Create block
		block := util.NewBeaconBlock()
		block.Block.Slot = 1
		block.Block.ProposerIndex = 0
		sig, err := signing.ComputeDomainAndSign(beaconState, 0, block.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		block.Signature = sig

		signedBlock, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)

		slashingPool := &slashingsmock.PoolMock{}
		chainService := &mock.ChainService{
			State:   beaconState,
			Genesis: time.Now(),
			Block:   signedBlock,
		}

		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: slashingPool,
			},
			seenBlockCache: lruwrpr.New(10),
		}

		err = r.detectAndBroadcastEquivocation(ctx, signedBlock)
		require.NoError(t, err)
		assert.Equal(t, 0, len(slashingPool.PendingPropSlashings), "Expected no slashings for same signature")
	})

	t.Run("head state error", func(t *testing.T) {
		block := util.NewBeaconBlock()
		block.Block.Slot = 1
		block.Block.ProposerIndex = 0
		block.Block.ParentRoot = bytesutil.PadTo([]byte("parent1"), 32)
		sig1, err := signing.ComputeDomainAndSign(beaconState, 0, block.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		block.Signature = sig1

		headBlock := util.NewBeaconBlock()
		headBlock.Block.Slot = 1                                            // Same slot
		headBlock.Block.ProposerIndex = 0                                   // Same proposer
		headBlock.Block.ParentRoot = bytesutil.PadTo([]byte("parent2"), 32) // Different parent root
		sig2, err := signing.ComputeDomainAndSign(beaconState, 0, headBlock.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		headBlock.Signature = sig2

		signedBlock, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)

		signedHeadBlock, err := blocks.NewSignedBeaconBlock(headBlock)
		require.NoError(t, err)

		chainService := &mock.ChainService{
			State:        nil,
			Block:        signedHeadBlock,
			HeadStateErr: errors.New("could not get head state"),
		}

		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: &slashingsmock.PoolMock{},
			},
			seenBlockCache: lruwrpr.New(10),
		}

		err = r.detectAndBroadcastEquivocation(ctx, signedBlock)
		require.ErrorContains(t, "could not get head state", err)
	})
	t.Run("signature verification failure", func(t *testing.T) {
		// Create head block
		headBlock := util.NewBeaconBlock()
		headBlock.Block.Slot = 1
		headBlock.Block.ProposerIndex = 0
		sig1, err := signing.ComputeDomainAndSign(beaconState, 0, headBlock.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		headBlock.Signature = sig1

		// Create test block with invalid signature
		newBlock := util.NewBeaconBlock()
		newBlock.Block.Slot = 1
		newBlock.Block.ProposerIndex = 0
		newBlock.Block.ParentRoot = bytesutil.PadTo([]byte("different"), 32)
		// generate invalid signature
		invalidSig := make([]byte, 96)
		copy(invalidSig, []byte("invalid signature"))
		newBlock.Signature = invalidSig

		signedHeadBlock, err := blocks.NewSignedBeaconBlock(headBlock)
		require.NoError(t, err)
		signedNewBlock, err := blocks.NewSignedBeaconBlock(newBlock)
		require.NoError(t, err)

		slashingPool := &slashingsmock.PoolMock{}
		chainService := &mock.ChainService{
			State:   beaconState,
			Genesis: time.Now(),
			Block:   signedHeadBlock,
		}

		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: slashingPool,
			},
			seenBlockCache: lruwrpr.New(10),
		}

		err = r.detectAndBroadcastEquivocation(ctx, signedNewBlock)
		require.ErrorIs(t, err, ErrSlashingSignatureFailure)
	})
}
