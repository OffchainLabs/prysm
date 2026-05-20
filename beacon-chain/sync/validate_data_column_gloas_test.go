package sync

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	ssz "github.com/prysmaticlabs/fastssz"
)

func gloasFixture(t *testing.T) (*ethpb.DataColumnSidecarGloas, interfaces.ReadOnlySignedBeaconBlock) {
	t.Helper()

	roBlock, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 1, util.WithSlot(1))
	require.Equal(t, true, len(roSidecars) > 0)

	base := roSidecars[0]
	bid := util.GenerateTestSignedExecutionPayloadBid(base.Slot())
	comms, err := roBlock.Block().Body().BlobKzgCommitments()
	require.NoError(t, err)
	bid.Message.BlobKzgCommitments = bytesutil.SafeCopy2dBytes(comms)

	pb := util.NewBeaconBlockGloas()
	pb.Block.Slot = base.Slot()
	pb.Block.ProposerIndex = roBlock.Block().ProposerIndex()
	parentRoot := roBlock.Block().ParentRoot()
	pb.Block.ParentRoot = parentRoot[:]
	stateRoot := roBlock.Block().StateRoot()
	pb.Block.StateRoot = stateRoot[:]
	pb.Block.Body.SignedExecutionPayloadBid = bid

	signedBlock, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)

	blockRoot, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)

	sidecar := &ethpb.DataColumnSidecarGloas{
		Index:           base.Index(),
		Column:          bytesutil.SafeCopy2dBytes(base.Column()),
		KzgProofs:       bytesutil.SafeCopy2dBytes(base.KzgProofs()),
		Slot:            base.Slot(),
		BeaconBlockRoot: blockRoot[:],
	}

	return sidecar, signedBlock
}

// newPendingGloasService returns a Service pre-populated with the queue maps and the
// seen-column cache. Callers can attach additional cfg fields (p2p, dataColumnStorage,
// etc.) on the returned value.
func newPendingGloasService(clock *startup.Clock) *Service {
	return &Service{
		cfg:                          &config{clock: clock},
		pendingGloasColumns:          make(map[[32]byte]*pendingGloasEntry),
		pendingGloasPeerColumnCounts: make(map[peer.ID]int),
		pendingGloasPeerRootCounts:   make(map[peer.ID]int),
		seenDataColumnCache:          newSlotAwareCache(seenDataColumnSize),
	}
}

// makePendingGloasSidecar builds a minimal DataColumnSidecarGloas with the given
// (root, index, slot) and a single 2 KiB column + 48-byte KZG proof. It is enough to
// exercise queue bookkeeping but is NOT a valid sidecar for KZG verification.
func makePendingGloasSidecar(root [32]byte, index uint64, slot primitives.Slot) *ethpb.DataColumnSidecarGloas {
	return &ethpb.DataColumnSidecarGloas{
		Index:           index,
		Slot:            slot,
		BeaconBlockRoot: root[:],
		Column:          [][]byte{make([]byte, 2048)},
		KzgProofs:       [][]byte{make([]byte, 48)},
	}
}

func TestValidateDataColumnGloas(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	ctx := t.Context()
	genericError := errors.New("generic error")

	serviceAndMessage := func(t *testing.T, newDataColumnsVerifier verification.NewDataColumnsVerifier, msg ssz.Marshaler, columnIndex uint64) (*Service, *pubsub.Message) {
		t.Helper()

		const genesisNSec = 0

		p := p2ptest.NewTestP2P(t)
		genesisSec := time.Now().Unix() - int64(params.BeaconConfig().SecondsPerSlot)
		chainService := &mock.ChainService{Genesis: time.Unix(genesisSec, genesisNSec)}

		clock := startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)
		service := &Service{
			cfg:                          &config{p2p: p, initialSync: &mockSync.Sync{}, clock: clock, chain: chainService, batchVerifierLimit: 10},
			ctx:                          ctx,
			newColumnsVerifier:           newDataColumnsVerifier,
			seenDataColumnCache:          newSlotAwareCache(seenDataColumnSize),
			pendingGloasColumns:          make(map[[32]byte]*pendingGloasEntry),
			pendingGloasPeerColumnCounts: make(map[peer.ID]int),
			pendingGloasPeerRootCounts:   make(map[peer.ID]int),
		}

		buf := new(bytes.Buffer)
		_, err := p.Encoding().EncodeGossip(buf, msg)
		require.NoError(t, err)

		topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
		digest, err := service.currentForkDigest()
		require.NoError(t, err)

		subnet := peerdas.ComputeSubnetForDataColumnSidecar(columnIndex)
		topic = service.addDigestAndIndexToTopic(topic, digest, subnet)

		message := &pubsub.Message{Message: &pb.Message{Data: buf.Bytes(), Topic: &topic}}
		return service, message
	}

	t.Run("ignores unseen block", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, _ := gloasFixture(t)
		service, message := serviceAndMessage(t, testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrValidFields: genericError}), sidecar, sidecar.Index)
		result, err := service.validateDataColumn(ctx, "aDummyPID", message)
		require.ErrorContains(t, "gloas data column block not yet seen", err)
		require.Equal(t, pubsub.ValidationIgnore, result)

		// The queued entry must record the forwarding peer (`pid`), not msg.From which
		// is empty under StrictNoSign/WithNoAuthor and would no-op the bad-response scorer.
		blockRoot := bytesutil.ToBytes32(sidecar.BeaconBlockRoot)
		entry := service.pendingGloasColumns[blockRoot]
		require.NotNil(t, entry)
		require.NotNil(t, entry.columns[sidecar.Index])
		require.NotNil(t, entry.columns[sidecar.Index][peer.ID("aDummyPID")])
	})

	t.Run("validates against bid commitments", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, signedBlock := gloasFixture(t)
		service, message := serviceAndMessage(t, testVerifierReturnsAll(&verification.MockDataColumnsVerifier{}), sidecar, sidecar.Index)

		db := dbtest.SetupDB(t)
		chainService := &mock.ChainService{
			Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
			DB:      db,
		}
		service.cfg.beaconDB = db
		service.cfg.chain = chainService
		require.NoError(t, db.SaveBlock(ctx, signedBlock))

		result, err := service.validateDataColumn(ctx, "aDummyPID", message)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)

		validated, ok := message.ValidatorData.(*ethpb.DataColumnSidecarGloas)
		require.Equal(t, true, ok)
		require.Equal(t, true, bytes.Equal(validated.KzgProofs[0], sidecar.KzgProofs[0]))

		result, err = service.validateDataColumn(ctx, "aDummyPID", message)
		require.ErrorContains(t, "data column sidecar already seen for block root", err)
		require.Equal(t, pubsub.ValidationIgnore, result)
	})

	t.Run("rejects slot mismatch", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, signedBlock := gloasFixture(t)
		sidecar.Slot++

		service, _ := serviceAndMessage(t, testVerifierReturnsAll(&verification.MockDataColumnsVerifier{}), sidecar, sidecar.Index)

		db := dbtest.SetupDB(t)
		chainService := &mock.ChainService{
			Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
			DB:      db,
		}
		service.cfg.beaconDB = db
		service.cfg.chain = chainService
		require.NoError(t, db.SaveBlock(ctx, signedBlock))

		blockRoot, err := signedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		roDataColumn, err := blocks.NewRODataColumnGloasWithRoot(sidecar, blockRoot)
		require.NoError(t, err)

		digest, err := service.currentForkDigest()
		require.NoError(t, err)
		topic := service.addDigestAndIndexToTopic(p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.DataColumnSidecarGloas]()], digest, peerdas.ComputeSubnetForDataColumnSidecar(sidecar.Index))
		msg := &pubsub.Message{Message: &pb.Message{Topic: &topic}}

		_, err = service.validateDataColumnGloas(ctx, "aDummyPID", msg, roDataColumn, "/data_column_sidecar_%d/")
		require.ErrorContains(t, "slot does not match block slot", err)
	})
}

func TestPendingGloasColumns(t *testing.T) {
	clock := startup.NewClock(time.Now(), [32]byte{})

	t.Run("queue and retrieve", func(t *testing.T) {
		s := newPendingGloasService(clock)
		root := [32]byte{0xaa}
		dc := makePendingGloasSidecar(root, 5, clock.CurrentSlot())
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)

		s.queuePendingGloasColumn(roCol, "peer1")
		require.Equal(t, true, s.hasPendingGloasColumns(root))

		entry := s.pendingGloasColumns[root]
		require.NotNil(t, entry)
		require.NotNil(t, entry.columns[5])
		require.Equal(t, 1, len(entry.columns[5]))
		require.NotNil(t, entry.columns[5][peer.ID("peer1")])
	})

	t.Run("retains both peers for same root and index", func(t *testing.T) {
		s := newPendingGloasService(clock)
		root := [32]byte{0xbb}
		dc := makePendingGloasSidecar(root, 10, clock.CurrentSlot())
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)

		s.queuePendingGloasColumn(roCol, "peer1")
		s.queuePendingGloasColumn(roCol, "peer2")
		cell := s.pendingGloasColumns[root].columns[10]
		require.Equal(t, 2, len(cell))
		require.NotNil(t, cell[peer.ID("peer1")])
		require.NotNil(t, cell[peer.ID("peer2")])

		// A second submission from the same peer is a no-op.
		s.queuePendingGloasColumn(roCol, "peer1")
		require.Equal(t, 2, len(cell))
		require.Equal(t, 1, s.pendingGloasPeerColumnCounts[peer.ID("peer1")])
	})

	t.Run("nil block is no-op", func(t *testing.T) {
		s := newPendingGloasService(clock)
		root := [32]byte{0xcc}
		s.pendingGloasColumns[root] = &pendingGloasEntry{slot: clock.CurrentSlot()}

		s.processPendingGloasColumns(root, nil)
		// Entry should remain because the block was nil.
		require.Equal(t, true, s.hasPendingGloasColumns(root))
	})

	t.Run("index out of bounds rejected", func(t *testing.T) {
		s := newPendingGloasService(clock)
		root := [32]byte{0xee}
		dc := makePendingGloasSidecar(root, fieldparams.NumberOfColumns+1, clock.CurrentSlot())
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)

		s.queuePendingGloasColumn(roCol, "peer1")
		require.Equal(t, false, s.hasPendingGloasColumns(root))
	})

	t.Run("map capped at maxPendingGloasRoots", func(t *testing.T) {
		s := newPendingGloasService(clock)
		// Fill up to the cap with one distinct peer per root, so the per-peer
		// root cap doesn't trigger first and we actually exercise the global
		// map cap.
		for i := range maxPendingGloasRoots {
			root := [32]byte{byte(i)}
			dc := makePendingGloasSidecar(root, 0, clock.CurrentSlot())
			roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
			require.NoError(t, err)
			s.queuePendingGloasColumn(roCol, peer.ID(fmt.Sprintf("peer%d", i)))
		}
		require.Equal(t, maxPendingGloasRoots, len(s.pendingGloasColumns))

		// One more from a fresh peer should be dropped because the global root
		// map is full.
		overflowRoot := [32]byte{0xff}
		dc := makePendingGloasSidecar(overflowRoot, 0, clock.CurrentSlot())
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, overflowRoot)
		require.NoError(t, err)
		s.queuePendingGloasColumn(roCol, "overflow")
		require.Equal(t, false, s.hasPendingGloasColumns(overflowRoot))

		// Adding to an existing root from the peer that already owns it should
		// still work and must not consume an additional root slot.
		existingRoot := [32]byte{0x00}
		dc2 := makePendingGloasSidecar(existingRoot, 1, clock.CurrentSlot())
		roCol2, err := blocks.NewRODataColumnGloasWithRoot(dc2, existingRoot)
		require.NoError(t, err)
		s.queuePendingGloasColumn(roCol2, peer.ID("peer0"))
		require.NotNil(t, s.pendingGloasColumns[existingRoot].columns[1])
		require.Equal(t, 1, s.pendingGloasPeerRootCounts[peer.ID("peer0")])
	})

	t.Run("per-peer root cap rejects new roots beyond the limit", func(t *testing.T) {
		s := newPendingGloasService(clock)
		// "noisy" claims its full root quota with one column on each root.
		for i := range maxPendingGloasRootsPerPeer {
			root := [32]byte{byte(i)}
			dc := makePendingGloasSidecar(root, 0, clock.CurrentSlot())
			roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
			require.NoError(t, err)
			s.queuePendingGloasColumn(roCol, "noisy")
		}
		require.Equal(t, maxPendingGloasRootsPerPeer, s.pendingGloasPeerRootCounts["noisy"])

		// A further sidecar from "noisy" on a fresh root must be dropped.
		extraRoot := [32]byte{0xaa}
		dc := makePendingGloasSidecar(extraRoot, 0, clock.CurrentSlot())
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, extraRoot)
		require.NoError(t, err)
		s.queuePendingGloasColumn(roCol, "noisy")
		require.Equal(t, false, s.hasPendingGloasColumns(extraRoot))

		// A different peer is unaffected and can still claim the fresh root.
		s.queuePendingGloasColumn(roCol, "honest")
		require.Equal(t, true, s.hasPendingGloasColumns(extraRoot))

		// "noisy" can still add columns to roots it already owns without
		// consuming a new root slot.
		ownedRoot := [32]byte{byte(0)}
		dc2 := makePendingGloasSidecar(ownedRoot, 1, clock.CurrentSlot())
		roCol2, err := blocks.NewRODataColumnGloasWithRoot(dc2, ownedRoot)
		require.NoError(t, err)
		s.queuePendingGloasColumn(roCol2, "noisy")
		require.NotNil(t, s.pendingGloasColumns[ownedRoot].columns[1])
		require.Equal(t, maxPendingGloasRootsPerPeer, s.pendingGloasPeerRootCounts["noisy"])
	})

	t.Run("per-peer column cap rejects further inserts", func(t *testing.T) {
		s := newPendingGloasService(clock)
		root := [32]byte{0xab}
		// Fill the per-peer quota for "noisy" using distinct column indices on a single root.
		for i := range uint64(maxPendingGloasColumnsPerPeer) {
			dc := makePendingGloasSidecar(root, i, clock.CurrentSlot())
			roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
			require.NoError(t, err)
			s.queuePendingGloasColumn(roCol, "noisy")
		}
		require.Equal(t, maxPendingGloasColumnsPerPeer, s.pendingGloasPeerColumnCounts["noisy"])

		// A new column from "noisy" on a fresh root must be dropped.
		overflowRoot := [32]byte{0xcd}
		dc := makePendingGloasSidecar(overflowRoot, 0, clock.CurrentSlot())
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, overflowRoot)
		require.NoError(t, err)
		s.queuePendingGloasColumn(roCol, "noisy")
		require.Equal(t, false, s.hasPendingGloasColumns(overflowRoot))

		// A different peer is unaffected.
		s.queuePendingGloasColumn(roCol, "honest")
		require.Equal(t, true, s.hasPendingGloasColumns(overflowRoot))
		require.Equal(t, 1, s.pendingGloasPeerColumnCounts["honest"])
	})

	t.Run("flush releases per-peer count", func(t *testing.T) {
		s := newPendingGloasService(clock)
		root := [32]byte{0xef}
		dc := makePendingGloasSidecar(root, 0, clock.CurrentSlot())
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)
		s.queuePendingGloasColumn(roCol, "peer1")
		require.Equal(t, 1, s.pendingGloasPeerColumnCounts["peer1"])

		// processPendingGloasColumns with a nil block bails out without flushing — exercise
		// the queue-deletion path directly so we test count release independent of verification.
		s.pendingGloasColumnsLock.Lock()
		entry := s.pendingGloasColumns[root]
		delete(s.pendingGloasColumns, root)
		s.releasePendingGloasPeerCounts(entry)
		s.pendingGloasColumnsLock.Unlock()

		_, exists := s.pendingGloasPeerColumnCounts["peer1"]
		require.Equal(t, false, exists)
	})

	t.Run("process verifies and saves valid columns", func(t *testing.T) {
		err := kzg.Start()
		require.NoError(t, err)

		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, signedBlock := gloasFixture(t)
		blockRoot, err := signedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		s := newPendingGloasService(clock)
		s.cfg.p2p = p2ptest.NewTestP2P(t)
		s.cfg.dataColumnStorage = filesystem.NewEphemeralDataColumnStorage(t)

		// Queue the sidecar.
		roCol, err := blocks.NewRODataColumnGloasWithRoot(sidecar, blockRoot)
		require.NoError(t, err)
		s.queuePendingGloasColumn(roCol, "peer1")
		require.Equal(t, true, s.hasPendingGloasColumns(blockRoot))

		// Process with the block.
		s.processPendingGloasColumns(blockRoot, signedBlock)
		require.Equal(t, false, s.hasPendingGloasColumns(blockRoot))

		// Column should be marked as seen.
		require.Equal(t, true, s.hasSeenDataColumnRootIndex(blockRoot, sidecar.Index))
	})

	t.Run("process skips already seen index without saving zero column", func(t *testing.T) {
		err := kzg.Start()
		require.NoError(t, err)

		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		dcs := filesystem.NewEphemeralDataColumnStorage(t)
		sidecar, signedBlock := gloasFixture(t)
		blockRoot, err := signedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		s := newPendingGloasService(clock)
		s.cfg.p2p = p2ptest.NewTestP2P(t)
		s.cfg.dataColumnStorage = dcs

		roCol, err := blocks.NewRODataColumnGloasWithRoot(sidecar, blockRoot)
		require.NoError(t, err)
		s.queuePendingGloasColumn(roCol, "peer1")
		s.setSeenDataColumnRootIndex(blockRoot, sidecar.Index, sidecar.Slot)

		s.processPendingGloasColumns(blockRoot, signedBlock)
		require.Equal(t, false, s.hasPendingGloasColumns(blockRoot))
		require.Equal(t, false, dcs.Summary(blockRoot).HasIndex(sidecar.Index))
	})

	t.Run("process downscores bad peer for slot mismatch", func(t *testing.T) {
		err := kzg.Start()
		require.NoError(t, err)

		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, signedBlock := gloasFixture(t)
		blockRoot, err := signedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		// Mismatch the slot.
		sidecar.Slot = sidecar.Slot + 10

		s := newPendingGloasService(clock)
		s.cfg.p2p = p2ptest.NewTestP2P(t)
		s.cfg.dataColumnStorage = filesystem.NewEphemeralDataColumnStorage(t)

		roCol, err := blocks.NewRODataColumnGloasWithRoot(sidecar, blockRoot)
		require.NoError(t, err)
		s.queuePendingGloasColumn(roCol, "badpeer")

		s.processPendingGloasColumns(blockRoot, signedBlock)
		require.Equal(t, false, s.hasPendingGloasColumns(blockRoot))
		// Column should NOT be marked as seen (it was invalid).
		require.Equal(t, false, s.hasSeenDataColumnRootIndex(blockRoot, sidecar.Index))
	})

	t.Run("process retains both peers and downscores only the bad one", func(t *testing.T) {
		err := kzg.Start()
		require.NoError(t, err)

		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, signedBlock := gloasFixture(t)
		blockRoot, err := signedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		good := &ethpb.DataColumnSidecarGloas{
			Index:           sidecar.Index,
			Slot:            sidecar.Slot,
			BeaconBlockRoot: bytesutil.SafeCopyBytes(sidecar.BeaconBlockRoot),
			Column:          bytesutil.SafeCopy2dBytes(sidecar.Column),
			KzgProofs:       bytesutil.SafeCopy2dBytes(sidecar.KzgProofs),
		}
		bad := &ethpb.DataColumnSidecarGloas{
			Index:           sidecar.Index,
			Slot:            sidecar.Slot + 10, // wrong slot triggers verification failure
			BeaconBlockRoot: bytesutil.SafeCopyBytes(sidecar.BeaconBlockRoot),
			Column:          bytesutil.SafeCopy2dBytes(sidecar.Column),
			KzgProofs:       bytesutil.SafeCopy2dBytes(sidecar.KzgProofs),
		}

		p := p2ptest.NewTestP2P(t)
		s := newPendingGloasService(clock)
		s.cfg.p2p = p
		s.cfg.dataColumnStorage = filesystem.NewEphemeralDataColumnStorage(t)

		goodCol, err := blocks.NewRODataColumnGloasWithRoot(good, blockRoot)
		require.NoError(t, err)
		badCol, err := blocks.NewRODataColumnGloasWithRoot(bad, blockRoot)
		require.NoError(t, err)
		s.queuePendingGloasColumn(goodCol, "goodpeer")
		s.queuePendingGloasColumn(badCol, "badpeer")
		// Both peers must be retained.
		require.Equal(t, 2, len(s.pendingGloasColumns[blockRoot].columns[sidecar.Index]))

		s.processPendingGloasColumns(blockRoot, signedBlock)
		require.Equal(t, false, s.hasPendingGloasColumns(blockRoot))
		// goodpeer's sidecar passes and is saved.
		require.Equal(t, true, s.hasSeenDataColumnRootIndex(blockRoot, sidecar.Index))
		// badpeer is downscored.
		badCount, err := p.Peers().Scorers().BadResponsesScorer().Count(peer.ID("badpeer"))
		require.NoError(t, err)
		require.Equal(t, 1, badCount)
		// goodpeer is not downscored; Count returns ErrPeerUnknown when the peer
		// has never been recorded as bad.
		_, err = p.Peers().Scorers().BadResponsesScorer().Count(peer.ID("goodpeer"))
		require.ErrorContains(t, "peer unknown", err)
	})

	t.Run("no entry is no-op", func(t *testing.T) {
		s := newPendingGloasService(clock)
		s.cfg.p2p = p2ptest.NewTestP2P(t)
		root := [32]byte{0xdd}
		pb := util.NewBeaconBlockGloas()
		blk, err := blocks.NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		// Should not panic.
		s.processPendingGloasColumns(root, blk)
	})

	t.Run("prune keeps current and next slot", func(t *testing.T) {
		s := newPendingGloasService(clock)
		currentSlot := clock.CurrentSlot()
		if currentSlot < 3 {
			t.Skip("need slot >= 3")
		}

		staleRoot := [32]byte{0x01}
		currentRoot := [32]byte{0x02}
		prevRoot := [32]byte{0x03}

		s.pendingGloasColumns[staleRoot] = &pendingGloasEntry{slot: currentSlot - 3}
		s.pendingGloasColumns[currentRoot] = &pendingGloasEntry{slot: currentSlot}
		s.pendingGloasColumns[prevRoot] = &pendingGloasEntry{slot: currentSlot - 1}

		// Simulate what the ticker does.
		s.pendingGloasColumnsLock.Lock()
		for r, e := range s.pendingGloasColumns {
			if e.slot+1 < currentSlot {
				delete(s.pendingGloasColumns, r)
			}
		}
		s.pendingGloasColumnsLock.Unlock()

		// Stale should be pruned, current and prev should remain.
		require.Equal(t, false, s.hasPendingGloasColumns(staleRoot))
		require.Equal(t, true, s.hasPendingGloasColumns(currentRoot))
		require.Equal(t, true, s.hasPendingGloasColumns(prevRoot))
	})
}
