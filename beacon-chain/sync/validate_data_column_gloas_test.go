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
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	ssz "github.com/prysmaticlabs/fastssz"
)

func TestValidateDataColumnGloas(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	ctx := t.Context()
	genericError := errors.New("generic error")

	serviceAndMessage := func(t *testing.T, newDataColumnsVerifier verification.NewDataColumnsVerifier, msg ssz.Marshaler) (*Service, *pubsub.Message) {
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
		}

		buf := new(bytes.Buffer)
		_, err := p.Encoding().EncodeGossip(buf, msg)
		require.NoError(t, err)

		topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
		digest, err := service.currentForkDigest()
		require.NoError(t, err)

		subnet := peerdas.ComputeSubnetForDataColumnSidecar(msg.(*ethpb.DataColumnSidecar).Index)
		topic = service.addDigestAndIndexToTopic(topic, digest, subnet)

		message := &pubsub.Message{Message: &pb.Message{Data: buf.Bytes(), Topic: &topic}}
		return service, message
	}

	gloasFixture := func(t *testing.T) (*ethpb.DataColumnSidecar, interfaces.ReadOnlySignedBeaconBlock) {
		t.Helper()

		_, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 1, util.WithSlot(1))
		require.Equal(t, true, len(roSidecars) > 0)

		base := roSidecars[0]
		bid := util.GenerateTestSignedExecutionPayloadBid(base.Slot())
		bid.Message.BlobKzgCommitments = bytesutil.SafeCopy2dBytes(base.KzgCommitments)

		pb := util.NewBeaconBlockGloas()
		pb.Block.Slot = base.Slot()
		pb.Block.ProposerIndex = base.ProposerIndex()
		pb.Block.ParentRoot = bytes.Clone(base.SignedBlockHeader.Header.ParentRoot)
		pb.Block.StateRoot = bytes.Clone(base.SignedBlockHeader.Header.StateRoot)
		pb.Block.Body.SignedExecutionPayloadBid = bid

		signedBlock, err := blocks.NewSignedBeaconBlock(pb)
		require.NoError(t, err)

		header, err := signedBlock.Header()
		require.NoError(t, err)

		invalidCommitments := make([][]byte, len(base.KzgCommitments))
		for i := range invalidCommitments {
			invalidCommitments[i] = bytes.Repeat([]byte{0x42}, len(base.KzgCommitments[i]))
		}

		sidecar := &ethpb.DataColumnSidecar{
			Index:                        base.Index,
			Column:                       bytesutil.SafeCopy2dBytes(base.Column),
			KzgCommitments:               invalidCommitments,
			KzgProofs:                    bytesutil.SafeCopy2dBytes(base.KzgProofs),
			SignedBlockHeader:            header,
			KzgCommitmentsInclusionProof: bytesutil.SafeCopy2dBytes(base.KzgCommitmentsInclusionProof),
		}

		return sidecar, signedBlock
	}

	t.Run("ignores unseen block", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, _ := gloasFixture(t)
		service, message := serviceAndMessage(t, testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrValidFields: genericError}), sidecar)
		result, err := service.validateDataColumn(ctx, "aDummyPID", message)
		require.ErrorContains(t, "gloas data column block not yet seen", err)
		require.Equal(t, pubsub.ValidationIgnore, result)
	})

	t.Run("validates against bid commitments", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, signedBlock := gloasFixture(t)
		service, message := serviceAndMessage(t, testVerifierReturnsAll(&verification.MockDataColumnsVerifier{}), sidecar)

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

		validated, ok := message.ValidatorData.(blocks.VerifiedRODataColumn)
		require.Equal(t, true, ok)
		require.Equal(t, true, bytes.Equal(validated.KzgCommitments[0], sidecar.KzgCommitments[0]))

		result, err = service.validateDataColumn(ctx, "aDummyPID", message)
		require.ErrorContains(t, "data column sidecar already seen for block root", err)
		require.Equal(t, pubsub.ValidationIgnore, result)
	})

	t.Run("rejects slot mismatch", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		sidecar, signedBlock := gloasFixture(t)
		sidecar.SignedBlockHeader.Header.Slot++

		service, _ := serviceAndMessage(t, testVerifierReturnsAll(&verification.MockDataColumnsVerifier{}), sidecar)

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
		roDataColumn, err := blocks.NewRODataColumnWithRoot(sidecar, blockRoot)
		require.NoError(t, err)

		digest, err := service.currentForkDigest()
		require.NoError(t, err)
		topic := service.addDigestAndIndexToTopic(p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.DataColumnSidecar]()], digest, peerdas.ComputeSubnetForDataColumnSidecar(sidecar.Index))
		msg := &pubsub.Message{Message: &pb.Message{Topic: &topic}}

		_, err = service.validateDataColumnGloas(ctx, msg, roDataColumn, "/data_column_sidecar_%d/")
		require.ErrorContains(t, "slot does not match block slot", err)
	})
}
