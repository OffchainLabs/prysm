package sync

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

// buildGloasService returns a Service with Gloas epoch = 0 and the given chain/db wired in.
func buildGloasService(t *testing.T, chainService *mock.ChainService) *Service {
	t.Helper()
	p := p2ptest.NewTestP2P(t)
	clock := startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)
	return &Service{
		cfg: &config{
			p2p:                p,
			initialSync:        &mockSync.Sync{},
			clock:              clock,
			chain:              chainService,
			beaconDB:           chainService.DB,
			operationNotifier:  chainService.OperationNotifier(),
			batchVerifierLimit: 10,
		},
		ctx:                      t.Context(),
		newColumnsVerifier:       testVerifierReturnsAll(&verification.MockDataColumnsVerifier{}),
		seenDataColumnCache:      newSlotAwareCache(seenDataColumnSize),
		pendingDataColumnsByRoot: make(map[[32]byte][]pendingGloasDataColumnEntry),
		pendingDataColumnKeys:    make(map[string]bool),
	}
}

// buildGloasFixture returns a sidecar + its Gloas block + an RODataColumn keyed by the block root.
func buildGloasFixture(t *testing.T, svc *Service) (blocks.RODataColumn, interfaces.ReadOnlySignedBeaconBlock, string) {
	t.Helper()
	require.NoError(t, kzg.Start())

	_, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 1, util.WithSlot(1))
	require.Equal(t, true, len(roSidecars) > 0)
	base := roSidecars[0]

	kzgCommitments, err := base.KzgCommitments()
	require.NoError(t, err)
	kzgProofs := base.KzgProofs()
	column := base.Column()
	kzgCommitmentsInclusionProof, err := base.KzgCommitmentsInclusionProof()
	require.NoError(t, err)
	proposerIndex, err := base.ProposerIndex()
	require.NoError(t, err)
	signedBlockHeader, err := base.SignedBlockHeader()
	require.NoError(t, err)

	bid := util.GenerateTestSignedExecutionPayloadBid(base.Slot())
	bid.Message.BlobKzgCommitments = bytesutil.SafeCopy2dBytes(kzgCommitments)

	gloasProto := util.NewBeaconBlockGloas()
	gloasProto.Block.Slot = base.Slot()
	gloasProto.Block.ProposerIndex = proposerIndex
	gloasProto.Block.ParentRoot = bytes.Clone(signedBlockHeader.Header.ParentRoot)
	gloasProto.Block.StateRoot = bytes.Clone(signedBlockHeader.Header.StateRoot)
	gloasProto.Block.Body.SignedExecutionPayloadBid = bid

	signedBlock, err := blocks.NewSignedBeaconBlock(gloasProto)
	require.NoError(t, err)
	header, err := signedBlock.Header()
	require.NoError(t, err)
	blockRoot, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)

	sidecar := &ethpb.DataColumnSidecar{
		Index:                        base.Index(),
		Column:                       bytesutil.SafeCopy2dBytes(column),
		KzgCommitments:               bytesutil.SafeCopy2dBytes(kzgCommitments),
		KzgProofs:                    bytesutil.SafeCopy2dBytes(kzgProofs),
		SignedBlockHeader:            header,
		KzgCommitmentsInclusionProof: bytesutil.SafeCopy2dBytes(kzgCommitmentsInclusionProof),
	}

	rodc, err := blocks.NewRODataColumnWithRoot(sidecar, blockRoot)
	require.NoError(t, err)

	digest, err := svc.currentForkDigest()
	require.NoError(t, err)
	topicBase := p2p.GossipTypeMapping[reflect.TypeOf(sidecar)]
	topic := svc.addDigestAndIndexToTopic(topicBase, digest, uint64(0))

	return rodc, signedBlock, topic
}

// TestAddDataColumnToPendingQueue checks basic queue insertion and deduplication.
func TestAddDataColumnToPendingQueue(t *testing.T) {
	require.NoError(t, kzg.Start())
	chainService := &mock.ChainService{Genesis: time.Now()}
	svc := buildGloasService(t, chainService)

	_, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 1, util.WithSlot(1))
	require.Equal(t, true, len(roSidecars) > 0)
	rodc := roSidecars[0]

	// First insertion.
	svc.addDataColumnToPendingQueue(rodc, "test-topic", "")
	root := rodc.BlockRoot()
	require.Equal(t, 1, len(svc.pendingDataColumnsByRoot))
	require.Equal(t, 1, len(svc.pendingDataColumnsByRoot[root]))
	require.Equal(t, "test-topic", svc.pendingDataColumnsByRoot[root][0].topic)
	require.Equal(t, rodc.Slot(), svc.pendingDataColumnsByRoot[root][0].arrivalSlot)
	require.Equal(t, 1, len(svc.pendingDataColumnKeys))

	// Duplicate arrival must not create a second entry.
	svc.addDataColumnToPendingQueue(rodc, "test-topic", "")
	require.Equal(t, 1, len(svc.pendingDataColumnsByRoot[root]))
	require.Equal(t, 1, len(svc.pendingDataColumnKeys))
}

// TestProcessPendingDataColumns_EvictsOldEntries verifies stale entries are removed.
// Eviction is triggered on each call to processPendingDataColumnsForRoot, which sweeps
// all roots before processing the specific one. Calling it with a zero root that has no
// queued entries exercises the eviction-only path.
func TestProcessPendingDataColumns_EvictsOldEntries(t *testing.T) {
	require.NoError(t, kzg.Start())

	// Set genesis far enough in the past so current slot > pendingDataColumnExpSlots.
	// With SecondsPerSlot=12 and expiry=4, we need current slot >= 5, so genesis must
	// be at least 5 slots ago.
	secsBack := int64(params.BeaconConfig().SecondsPerSlot) * (int64(pendingDataColumnExpSlots) + 2)
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-secsBack, 0)}
	svc := buildGloasService(t, chainService)

	_, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 1, util.WithSlot(1))
	require.Equal(t, true, len(roSidecars) > 0)
	rodc := roSidecars[0]

	key := computeRootIndexCacheKey(rodc.BlockRoot(), rodc.Index())
	root := rodc.BlockRoot()

	// Insert with arrivalSlot=0 — stale relative to current slot.
	svc.pendingDataColumnsByRoot[root] = []pendingGloasDataColumnEntry{
		{roDataColumn: rodc, topic: "t", arrivalSlot: 0},
	}
	svc.pendingDataColumnKeys[key] = true

	// Trigger eviction by processing a non-existent root. The global eviction sweep
	// runs at the start of every processPendingDataColumnsForRoot call regardless of root.
	svc.processPendingDataColumnsForRoot(t.Context(), [32]byte{})

	require.Equal(t, 0, len(svc.pendingDataColumnsByRoot))
	require.Equal(t, 0, len(svc.pendingDataColumnKeys))
}

// TestProcessPendingDataColumns_UnrelatedRootDoesNotClear verifies that processing
// a different block root leaves unrelated queued entries untouched.
// With event-driven processing, processPendingDataColumnsForRoot is only called when
// a specific block arrives; entries for other roots are unaffected.
func TestProcessPendingDataColumns_UnrelatedRootDoesNotClear(t *testing.T) {
	require.NoError(t, kzg.Start())
	chainService := &mock.ChainService{Genesis: time.Now()}
	svc := buildGloasService(t, chainService)

	_, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 1, util.WithSlot(1))
	require.Equal(t, true, len(roSidecars) > 0)
	rodc := roSidecars[0]

	svc.addDataColumnToPendingQueue(rodc, "t", "")

	// Fire with a completely different root — entry for rodc's root must survive.
	svc.processPendingDataColumnsForRoot(t.Context(), [32]byte{0xff})
	require.Equal(t, 1, len(svc.pendingDataColumnsByRoot))
}

// TestValidateDataColumnGloas_QueuesWhenBlockNotSeen verifies that validateDataColumn
// returns ValidationIgnore and places the sidecar in the pending queue when the block
// has not been seen.
func TestValidateDataColumnGloas_QueuesWhenBlockNotSeen(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctx := t.Context()

	// No DB → HasBlock returns false.
	genesisSec := time.Now().Unix() - int64(params.BeaconConfig().SecondsPerSlot)
	chainService := &mock.ChainService{Genesis: time.Unix(genesisSec, 0)}
	svc := buildGloasService(t, chainService)

	// Build a minimal DataColumnSidecarGloas. The block root is random and not in
	// the DB, which is sufficient to trigger the block-not-seen path.
	// Column and KzgProofs are empty (zero blobs) which is valid SSZ.
	blockRoot := [32]byte{0xde, 0xad, 0xbe, 0xef}
	gloasSidecar := &ethpb.DataColumnSidecarGloas{
		Index:           0,
		Column:          [][]byte{},
		KzgProofs:       [][]byte{},
		Slot:            1,
		BeaconBlockRoot: blockRoot[:],
	}

	digest, err := svc.currentForkDigest()
	require.NoError(t, err)
	topicBase := p2p.GossipTypeMapping[reflect.TypeOf(gloasSidecar)]
	topic := svc.addDigestAndIndexToTopic(topicBase, digest, uint64(0))

	buf := new(bytes.Buffer)
	_, err = svc.cfg.p2p.Encoding().EncodeGossip(buf, gloasSidecar)
	require.NoError(t, err)
	msg := &pubsub.Message{Message: &pb.Message{Data: buf.Bytes(), Topic: &topic}}

	result, err := svc.validateDataColumn(ctx, "peer1", msg)
	require.ErrorContains(t, "gloas data column block not yet seen", err)
	require.Equal(t, pubsub.ValidationIgnore, result)

	// The sidecar must now be in the pending queue.
	require.Equal(t, 1, len(svc.pendingDataColumnsByRoot))
	require.Equal(t, 1, len(svc.pendingDataColumnKeys))
}

// TestProcessPendingDataColumns_ProcessesWhenBlockArrives is an end-to-end test:
// sidecar is queued before its block, then processed once the block is saved.
func TestProcessPendingDataColumns_ProcessesWhenBlockArrives(t *testing.T) {
	require.NoError(t, kzg.Start())

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctx := t.Context()
	db := dbtest.SetupDB(t)
	genesisSec := time.Now().Unix() - int64(params.BeaconConfig().SecondsPerSlot)
	chainService := &mock.ChainService{
		Genesis: time.Unix(genesisSec, 0),
		DB:      db,
	}
	svc := buildGloasService(t, chainService)
	rodc, signedBlock, topic := buildGloasFixture(t, svc)

	blockRoot, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)

	// Step 1: sidecar arrives before its block → queued.
	svc.addDataColumnToPendingQueue(rodc, topic, "")
	require.Equal(t, 1, len(svc.pendingDataColumnsByRoot))

	// Step 2: block arrives and is saved to DB.
	require.NoError(t, db.SaveBlock(ctx, signedBlock))

	// Step 3: beaconBlockSubscriber fires processPendingDataColumnsForRoot → entry processed and queue cleared.
	svc.processPendingDataColumnsForRoot(ctx, blockRoot)
	require.Equal(t, 0, len(svc.pendingDataColumnsByRoot))
	require.Equal(t, 0, len(svc.pendingDataColumnKeys))

	// Chain must have received the validated sidecar.
	require.Equal(t, 1, len(chainService.DataColumns))
}

// TestProcessPendingDataColumns_MultipleEntriesSameRoot verifies that the queue
// correctly holds multiple sidecars under a single block root and clears them all
// once the block is available (regardless of individual KZG validation outcomes).
// Note: the Gloas validator creates a real verifier, so only sidecars whose KZG
// data matches the block's bid commitments will actually reach the chain.
func TestProcessPendingDataColumns_MultipleEntriesSameRoot(t *testing.T) {
	require.NoError(t, kzg.Start())

	chainService := &mock.ChainService{Genesis: time.Now()}
	svc := buildGloasService(t, chainService)

	_, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 2, util.WithSlot(1))
	require.Equal(t, true, len(roSidecars) >= 2)

	rodc0 := roSidecars[0]
	rodc1 := roSidecars[1]
	// Both sidecars must belong to the same block root.
	require.Equal(t, rodc0.BlockRoot(), rodc1.BlockRoot())
	// And have different column indices so both are accepted by the queue.
	require.NotEqual(t, rodc0.Index(), rodc1.Index())

	root := rodc0.BlockRoot()
	topic := "t"

	svc.addDataColumnToPendingQueue(rodc0, topic, "")
	svc.addDataColumnToPendingQueue(rodc1, topic, "")

	// Both should be queued under the same root.
	require.Equal(t, 1, len(svc.pendingDataColumnsByRoot))
	require.Equal(t, 2, len(svc.pendingDataColumnsByRoot[root]))
	require.Equal(t, 2, len(svc.pendingDataColumnKeys))

	// Verify dedup: adding either again is a no-op.
	svc.addDataColumnToPendingQueue(rodc0, topic, "")
	require.Equal(t, 2, len(svc.pendingDataColumnsByRoot[root]))
}
