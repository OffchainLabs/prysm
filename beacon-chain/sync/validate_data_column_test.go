package sync

import (
	"bytes"
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
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
)

func TestValidateDataColumn(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	ctx := t.Context()

	t.Run("from self", func(t *testing.T) {
		p := p2ptest.NewTestP2P(t)
		s := &Service{cfg: &config{p2p: p}}

		result, err := s.validateDataColumn(ctx, s.cfg.p2p.PeerID(), nil)
		require.NoError(t, err)
		require.Equal(t, result, pubsub.ValidationAccept)
	})

	t.Run("syncing", func(t *testing.T) {
		p := p2ptest.NewTestP2P(t)
		s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{IsSyncing: true}}}

		result, err := s.validateDataColumn(ctx, "", nil)
		require.NoError(t, err)
		require.Equal(t, result, pubsub.ValidationIgnore)
	})

	t.Run("invalid topic", func(t *testing.T) {
		p := p2ptest.NewTestP2P(t)
		s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{}}}

		result, err := s.validateDataColumn(ctx, "", &pubsub.Message{Message: &pb.Message{}})
		require.ErrorIs(t, p2p.ErrInvalidTopic, err)
		require.Equal(t, result, pubsub.ValidationReject)
	})

	serviceAndMessage := func(t *testing.T, newDataColumnsVerifier verification.NewDataColumnsVerifier, msg ssz.Marshaler) (*Service, *pubsub.Message) {
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

		// Encode a `beaconBlock` message instead of expected.
		buf := new(bytes.Buffer)
		_, err := p.Encoding().EncodeGossip(buf, msg)
		require.NoError(t, err)

		topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
		digest, err := service.currentForkDigest()
		require.NoError(t, err)

		if dc, ok := msg.(*ethpb.DataColumnSidecar); ok {
			subnet := peerdas.ComputeSubnetForDataColumnSidecar(dc.Index)
			topic = service.addDigestAndIndexToTopic(topic, digest, subnet)
		} else {
			topic = service.addDigestToTopic(topic, digest)
		}

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

	t.Run("invalid message type", func(t *testing.T) {
		// Encode a `beaconBlock` message instead of expected.
		service, message := serviceAndMessage(t, nil, util.NewBeaconBlock())
		result, err := service.validateDataColumn(ctx, "", message)
		require.ErrorIs(t, errWrongMessage, err)
		require.Equal(t, pubsub.ValidationReject, result)
	})

	genericError := errors.New("generic error")

	dataColumnSidecarMsg := &ethpb.DataColumnSidecar{
		SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
			Header: &ethpb.BeaconBlockHeader{
				ParentRoot: make([]byte, fieldparams.RootLength),
				StateRoot:  make([]byte, fieldparams.RootLength),
				BodyRoot:   make([]byte, fieldparams.RootLength),
			},
			Signature: make([]byte, fieldparams.BLSSignatureLength),
		},
		KzgCommitmentsInclusionProof: [][]byte{
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
		},
	}

	testCases := []struct {
		name           string
		verifier       verification.NewDataColumnsVerifier
		expectedResult pubsub.ValidationResult
		expectedError  error
	}{
		{
			name:           "valid fields",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrValidFields: genericError}),
			expectedResult: pubsub.ValidationReject,
			expectedError:  genericError,
		},
		{
			name:           "correct subnet",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrCorrectSubnet: genericError}),
			expectedResult: pubsub.ValidationReject,
			expectedError:  genericError,
		},
		{
			name:           "not for future slot",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrNotFromFutureSlot: genericError}),
			expectedResult: pubsub.ValidationIgnore,
			expectedError:  genericError,
		},
		{
			name:           "slot above finalized",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrSlotAboveFinalized: genericError}),
			expectedResult: pubsub.ValidationIgnore,
			expectedError:  genericError,
		},
		{
			name:           "sidecar parent seen",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrSidecarParentSeen: genericError}),
			expectedResult: pubsub.ValidationIgnore,
			expectedError:  genericError,
		},
		{
			name:           "sidecar parent valid",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrSidecarParentValid: genericError}),
			expectedResult: pubsub.ValidationReject,
			expectedError:  genericError,
		},
		{
			name:           "valid proposer signature",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrValidProposerSignature: genericError}),
			expectedResult: pubsub.ValidationReject,
			expectedError:  genericError,
		},
		{
			name:           "sidecar parent slot lower",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrSidecarParentSlotLower: genericError}),
			expectedResult: pubsub.ValidationReject,
			expectedError:  genericError,
		},
		{
			name:           "sidecar descends from finalized",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrSidecarDescendsFromFinalized: genericError}),
			expectedResult: pubsub.ValidationReject,
			expectedError:  genericError,
		},
		{
			name:           "sidecar inclusion proven",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrSidecarInclusionProven: genericError}),
			expectedResult: pubsub.ValidationReject,
			expectedError:  genericError,
		},
		{
			name:           "sidecar proposer expected",
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrSidecarProposerExpected: genericError}),
			expectedResult: pubsub.ValidationReject,
			expectedError:  genericError,
		},
		{
			name:           "nominal",
			verifier:       testVerifierReturnsAll(&verification.MockDataColumnsVerifier{}),
			expectedResult: pubsub.ValidationAccept,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service, message := serviceAndMessage(t, tc.verifier, dataColumnSidecarMsg)
			result, err := service.validateDataColumn(ctx, "aDummyPID", message)
			require.ErrorIs(t, err, tc.expectedError)
			require.Equal(t, tc.expectedResult, result)
		})
	}

	t.Run("seen data column", func(t *testing.T) {
		service, message := serviceAndMessage(t, testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{}), dataColumnSidecarMsg)
		service.setSeenDataColumnIndex(0, 0, 0)
		result, err := service.validateDataColumn(ctx, "aDummyPID", message)
		require.ErrorContains(t, "data column sidecar already seen", err)
		require.Equal(t, pubsub.ValidationIgnore, result)
	})

	t.Run("gloas ignores unseen block", func(t *testing.T) {
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

	t.Run("gloas validates against bid commitments", func(t *testing.T) {
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

	t.Run("gloas rejects slot mismatch", func(t *testing.T) {
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

func testNewDataColumnSidecarsVerifier(verifier verification.MockDataColumnsVerifier) verification.NewDataColumnsVerifier {
	return func([]blocks.RODataColumn, []verification.Requirement) verification.DataColumnsVerifier {
		return &verifier
	}
}

func testVerifierReturnsAll(v *verification.MockDataColumnsVerifier) verification.NewDataColumnsVerifier {
	return func(cols []blocks.RODataColumn, reqs []verification.Requirement) verification.DataColumnsVerifier {
		for _, col := range cols {
			v.AppendRODataColumns(col)
		}
		return v
	}
}
