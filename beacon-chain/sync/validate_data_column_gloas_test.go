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

	gloasFixture := func(t *testing.T) (*ethpb.DataColumnSidecarGloas, interfaces.ReadOnlySignedBeaconBlock) {
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

		_, err = service.validateDataColumnGloas(ctx, msg, roDataColumn, "/data_column_sidecar_%d/")
		require.ErrorContains(t, "slot does not match block slot", err)
	})
}

func TestPendingGloasColumns(t *testing.T) {
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

		s.queuePendingGloasColumn(roCol, "peer1")
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

		s.queuePendingGloasColumn(roCol, "peer1")
		s.queuePendingGloasColumn(roCol, "peer2")
		require.Equal(t, peer.ID("peer1"), s.pendingGloasColumns[root].columns[10].peer)
	})

	t.Run("stale entries ignored on process", func(t *testing.T) {
		s := &Service{
			cfg:                 &config{clock: clock},
			pendingGloasColumns: make(map[[32]byte]*pendingGloasEntry),
		}
		staleRoot := [32]byte{0xcc}
		currentSlot := clock.CurrentSlot()
		if currentSlot == 0 {
			t.Skip("cannot test at slot 0")
		}
		s.pendingGloasColumns[staleRoot] = &pendingGloasEntry{slot: currentSlot - 1}

		s.processPendingGloasColumns(t.Context(), staleRoot, nil)
		require.Equal(t, false, s.hasPendingGloasColumns(staleRoot))
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

		s.queuePendingGloasColumn(roCol, "peer1")
		require.Equal(t, false, s.hasPendingGloasColumns(root))
	})
}
