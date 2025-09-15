package sync

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
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
			kzgChan:             make(chan *kzgVerifier, 100),
		}
		// Start the KZG verifier routine for batch verification
		go service.kzgVerifierRoutine()

		// Encode a `beaconBlock` message instead of expected.
		buf := new(bytes.Buffer)
		_, err := p.Encoding().EncodeGossip(buf, msg)
		require.NoError(t, err)

		topic := p2p.GossipTypeMapping[reflect.TypeOf(msg)]
		digest, err := service.currentForkDigest()
		require.NoError(t, err)

		topic = service.addDigestToTopic(topic, digest)

		message := &pubsub.Message{Message: &pb.Message{Data: buf.Bytes(), Topic: &topic}}

		return service, message
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
			verifier:       testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{}),
			expectedResult: pubsub.ValidationAccept,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service, message := serviceAndMessage(t, tc.verifier, dataColumnSidecarMsg)
			result, err := service.validateDataColumn(ctx, "aDummyPID", message)
			require.ErrorIs(t, tc.expectedError, err)
			require.Equal(t, tc.expectedResult, result)
		})
	}

	t.Run("seen data column", func(t *testing.T) {
		service, message := serviceAndMessage(t, testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{}), dataColumnSidecarMsg)
		service.setSeenDataColumnIndex(0, 0, 0)
		result, err := service.validateDataColumn(ctx, "aDummyPID", message)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationIgnore, result)
	})

}

func testNewDataColumnSidecarsVerifier(verifier verification.MockDataColumnsVerifier) verification.NewDataColumnsVerifier {
	return func([]blocks.RODataColumn, []verification.Requirement) verification.DataColumnsVerifier {
		return &verifier
	}
}
