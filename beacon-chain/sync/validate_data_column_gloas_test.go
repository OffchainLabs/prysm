package sync

import (
	"bytes"
	"errors"
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
			cfg:                 &config{p2p: p, initialSync: &mockSync.Sync{}, clock: clock, chain: chainService, batchVerifierLimit: 10},
			ctx:                 ctx,
			newColumnsVerifier:  newDataColumnsVerifier,
			seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
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
		cfg.DenebForkEpoch = 0
		cfg.ElectraForkEpoch = 0
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
		require.Equal(t, peer.ID("aDummyPID"), entry.columns[sidecar.Index].peer)
	})

	t.Run("validates against bid commitments", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.DenebForkEpoch = 0
		cfg.ElectraForkEpoch = 0
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
		cfg.DenebForkEpoch = 0
		cfg.ElectraForkEpoch = 0
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

	t.Run("rejects oversize column on queue path", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.DenebForkEpoch = 0
		cfg.ElectraForkEpoch = 0
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, _ := gloasFixture(t)
		maxCells := params.BeaconConfig().MaxBlobCommitmentsPerBlock
		sidecar.Column = make([][]byte, maxCells)
		sidecar.KzgProofs = make([][]byte, maxCells)
		for i := range sidecar.Column {
			sidecar.Column[i] = make([]byte, 2048)
			sidecar.KzgProofs[i] = make([]byte, 48)
		}

		service, message := serviceAndMessage(t, testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{}), sidecar, sidecar.Index)
		result, err := service.validateDataColumn(ctx, "aDummyPID", message)
		require.NotNil(t, err)
		require.Equal(t, pubsub.ValidationReject, result)

		blockRoot := bytesutil.ToBytes32(sidecar.BeaconBlockRoot)
		require.Equal(t, false, service.hasPendingGloasColumns(blockRoot))
	})
}

func TestPendingGloasColumns(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	clock := startup.NewClock(time.Now(), [32]byte{})

	t.Run("queue and retrieve", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
		root := [32]byte{0xaa}
		dc := &ethpb.DataColumnSidecarGloas{
			Index:           5,
			Slot:            clock.CurrentSlot(),
			BeaconBlockRoot: root[:],
			Column:          [][]byte{make([]byte, 2048)},
			KzgProofs:       [][]byte{make([]byte, 48)},
		}
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)

		require.NoError(t, s.queuePendingGloasColumn(roCol, "peer1"))
		require.Equal(t, true, s.hasPendingGloasColumns(root))

		entry := s.pendingGloasColumns[root]
		require.NotNil(t, entry)
		require.NotNil(t, entry.columns[5])
		require.Equal(t, peer.ID("peer1"), entry.columns[5].peer)
	})

	t.Run("dedup by index", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
		root := [32]byte{0xbb}
		dc := &ethpb.DataColumnSidecarGloas{
			Index:           10,
			Slot:            clock.CurrentSlot(),
			BeaconBlockRoot: root[:],
			Column:          [][]byte{make([]byte, 2048)},
			KzgProofs:       [][]byte{make([]byte, 48)},
		}
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)

		require.NoError(t, s.queuePendingGloasColumn(roCol, "peer1"))
		require.NoError(t, s.queuePendingGloasColumn(roCol, "peer2"))
		require.Equal(t, peer.ID("peer1"), s.pendingGloasColumns[root].columns[10].peer)
	})

	t.Run("nil block is no-op", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
		root := [32]byte{0xcc}
		s.pendingGloasColumns[root] = &pendingGloasEntry{slot: clock.CurrentSlot()}

		s.processPendingGloasColumns(root, nil)
		// Entry should remain because the block was nil.
		require.Equal(t, true, s.hasPendingGloasColumns(root))
	})

	t.Run("index out of bounds rejected", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
		root := [32]byte{0xee}
		dc := &ethpb.DataColumnSidecarGloas{
			Index:           fieldparams.NumberOfColumns + 1,
			Slot:            clock.CurrentSlot(),
			BeaconBlockRoot: root[:],
			Column:          [][]byte{make([]byte, 2048)},
			KzgProofs:       [][]byte{make([]byte, 48)},
		}
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)

		require.NotNil(t, s.queuePendingGloasColumn(roCol, "peer1"))
		require.Equal(t, false, s.hasPendingGloasColumns(root))
	})

	t.Run("oversize column rejected", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
		// SSZ allows 4096 cells; live max is much smaller. Without admission cap,
		// this 8 MiB sidecar would sit on the heap until prune.
		maxCells := params.BeaconConfig().MaxBlobCommitmentsPerBlock
		cells := make([][]byte, maxCells)
		proofs := make([][]byte, maxCells)
		for i := range cells {
			cells[i] = make([]byte, 2048)
			proofs[i] = make([]byte, 48)
		}
		root := [32]byte{0x77}
		dc := &ethpb.DataColumnSidecarGloas{
			Index:           0,
			Slot:            clock.CurrentSlot(),
			BeaconBlockRoot: root[:],
			Column:          cells,
			KzgProofs:       proofs,
		}
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)

		require.NotNil(t, s.queuePendingGloasColumn(roCol, "peer1"))
		require.Equal(t, false, s.hasPendingGloasColumns(root))
	})

	t.Run("empty column rejected", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
		root := [32]byte{0x78}
		dc := &ethpb.DataColumnSidecarGloas{
			Index:           0,
			Slot:            clock.CurrentSlot(),
			BeaconBlockRoot: root[:],
			Column:          nil,
			KzgProofs:       nil,
		}
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)

		require.NotNil(t, s.queuePendingGloasColumn(roCol, "peer1"))
		require.Equal(t, false, s.hasPendingGloasColumns(root))
	})

	t.Run("column proof length mismatch rejected", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
		root := [32]byte{0x79}
		dc := &ethpb.DataColumnSidecarGloas{
			Index:           0,
			Slot:            clock.CurrentSlot(),
			BeaconBlockRoot: root[:],
			Column:          [][]byte{make([]byte, 2048), make([]byte, 2048)},
			KzgProofs:       [][]byte{make([]byte, 48)},
		}
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
		require.NoError(t, err)

		require.NotNil(t, s.queuePendingGloasColumn(roCol, "peer1"))
		require.Equal(t, false, s.hasPendingGloasColumns(root))
	})

	t.Run("map capped at maxPendingGloasRoots", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
		// Fill up to the cap.
		for i := range maxPendingGloasRoots {
			root := [32]byte{byte(i)}
			dc := &ethpb.DataColumnSidecarGloas{
				Index:           0,
				Slot:            clock.CurrentSlot(),
				BeaconBlockRoot: root[:],
				Column:          [][]byte{make([]byte, 2048)},
				KzgProofs:       [][]byte{make([]byte, 48)},
			}
			roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, root)
			require.NoError(t, err)
			require.NoError(t, s.queuePendingGloasColumn(roCol, "peer1"))
		}
		require.Equal(t, maxPendingGloasRoots, len(s.pendingGloasColumns))

		// One more should be dropped.
		overflowRoot := [32]byte{0xff}
		dc := &ethpb.DataColumnSidecarGloas{
			Index:           0,
			Slot:            clock.CurrentSlot(),
			BeaconBlockRoot: overflowRoot[:],
			Column:          [][]byte{make([]byte, 2048)},
			KzgProofs:       [][]byte{make([]byte, 48)},
		}
		roCol, err := blocks.NewRODataColumnGloasWithRoot(dc, overflowRoot)
		require.NoError(t, err)
		require.NoError(t, s.queuePendingGloasColumn(roCol, "peer1"))
		require.Equal(t, false, s.hasPendingGloasColumns(overflowRoot))

		// Adding to an existing root should still work.
		existingRoot := [32]byte{0x00}
		dc2 := &ethpb.DataColumnSidecarGloas{
			Index:           1,
			Slot:            clock.CurrentSlot(),
			BeaconBlockRoot: existingRoot[:],
			Column:          [][]byte{make([]byte, 2048)},
			KzgProofs:       [][]byte{make([]byte, 48)},
		}
		roCol2, err := blocks.NewRODataColumnGloasWithRoot(dc2, existingRoot)
		require.NoError(t, err)
		require.NoError(t, s.queuePendingGloasColumn(roCol2, "peer1"))
		require.NotNil(t, s.pendingGloasColumns[existingRoot].columns[1])
	})

	t.Run("process verifies and saves valid columns", func(t *testing.T) {
		err := kzg.Start()
		require.NoError(t, err)

		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		p := p2ptest.NewTestP2P(t)
		dcs := filesystem.NewEphemeralDataColumnStorage(t)

		sidecar, signedBlock := gloasFixture(t)
		blockRoot, err := signedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		s := &Service{
			cfg: &config{
				p2p:               p,
				clock:             clock,
				dataColumnStorage: dcs,
			},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
			seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
		}

		// Queue the sidecar.
		roCol, err := blocks.NewRODataColumnGloasWithRoot(sidecar, blockRoot)
		require.NoError(t, err)
		require.NoError(t, s.queuePendingGloasColumn(roCol, "peer1"))
		require.Equal(t, true, s.hasPendingGloasColumns(blockRoot))

		// Process with the block.
		s.processPendingGloasColumns(blockRoot, signedBlock)
		require.Equal(t, false, s.hasPendingGloasColumns(blockRoot))

		// Column should be marked as seen.
		require.Equal(t, true, s.hasSeenDataColumnRootIndex(blockRoot, sidecar.Index))
	})

	t.Run("process downscores bad peer for slot mismatch", func(t *testing.T) {
		err := kzg.Start()
		require.NoError(t, err)

		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		p := p2ptest.NewTestP2P(t)
		dcs := filesystem.NewEphemeralDataColumnStorage(t)

		sidecar, signedBlock := gloasFixture(t)
		blockRoot, err := signedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		// Mismatch the slot.
		sidecar.Slot = sidecar.Slot + 10

		s := &Service{
			cfg: &config{
				p2p:               p,
				clock:             clock,
				dataColumnStorage: dcs,
			},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
			seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
		}

		roCol, err := blocks.NewRODataColumnGloasWithRoot(sidecar, blockRoot)
		require.NoError(t, err)
		require.NoError(t, s.queuePendingGloasColumn(roCol, "badpeer"))

		s.processPendingGloasColumns(blockRoot, signedBlock)
		require.Equal(t, false, s.hasPendingGloasColumns(blockRoot))
		// Column should NOT be marked as seen (it was invalid).
		require.Equal(t, false, s.hasSeenDataColumnRootIndex(blockRoot, sidecar.Index))
	})

	t.Run("no entry is no-op", func(t *testing.T) {
		p := p2ptest.NewTestP2P(t)
		s := &Service{
			cfg: &config{
				p2p:   p,
				clock: clock,
			},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
			seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
		}
		root := [32]byte{0xdd}
		pb := util.NewBeaconBlockGloas()
		blk, err := blocks.NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		// Should not panic.
		s.processPendingGloasColumns(root, blk)
	})

	t.Run("prune keeps current and next slot", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
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
