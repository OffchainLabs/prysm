package sync

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

func TestValidateDataColumn(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

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
		digest := service.currentForkDigest()

		if dc, ok := msg.(*ethpb.DataColumnSidecar); ok {
			subnet := peerdas.ComputeSubnetForDataColumnSidecar(dc.Index)
			topic = service.addDigestAndIndexToTopic(topic, digest, subnet)
		} else {
			topic = service.addDigestToTopic(topic, digest)
		}

		message := &pubsub.Message{Message: &pb.Message{Data: buf.Bytes(), Topic: &topic}}

		return service, message
	}

	t.Run("invalid message type", func(t *testing.T) {
		// Encode a `beaconBlock` message instead of expected.
		service, message := serviceAndMessage(t, nil, util.NewBeaconBlockFulu())
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
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationIgnore, result)
	})

	t.Run("unknown parent is ignored without a parent request", func(t *testing.T) {
		// A zero parent root reads as "being synced" to the chain mock, which would hide a fan-out.
		unknownParentMsg, ok := proto.Clone(dataColumnSidecarMsg).(*ethpb.DataColumnSidecar)
		require.Equal(t, true, ok)
		unknownParentMsg.SignedBlockHeader.Header.ParentRoot = bytesutil.PadTo([]byte("unknown parent"), fieldparams.RootLength)
		service, message := serviceAndMessage(t, testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrSidecarParentSeen: genericError}), unknownParentMsg)
		service.cfg.chain.(*mock.ChainService).FinalizedCheckPoint = &ethpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)}
		asked := blocksByRootProbe(t, service.cfg.p2p.(*p2ptest.TestP2P), service.cfg.clock)

		result, err := service.validateDataColumn(ctx, "aDummyPID", message)
		require.ErrorIs(t, err, genericError)
		require.Equal(t, pubsub.ValidationIgnore, result)
		requireNoBlocksByRootRequest(t, asked)
	})

	t.Run("fulu sidecar claiming gloas slot takes fulu path", func(t *testing.T) {
		gloasCfg := params.BeaconConfig().Copy()
		gloasCfg.GloasForkEpoch = 1
		params.OverrideBeaconConfig(gloasCfg)
		t.Cleanup(func() { params.OverrideBeaconConfig(cfg) })

		gloasSlotMsg, ok := proto.Clone(dataColumnSidecarMsg).(*ethpb.DataColumnSidecar)
		require.Equal(t, true, ok)
		gloasSlotMsg.SignedBlockHeader.Header.Slot = params.BeaconConfig().SlotsPerEpoch
		service, message := serviceAndMessage(t, testNewDataColumnSidecarsVerifier(verification.MockDataColumnsVerifier{ErrValidProposerSignature: genericError}), gloasSlotMsg)
		result, err := service.validateDataColumn(ctx, "aDummyPID", message)
		require.ErrorIs(t, err, genericError)
		require.Equal(t, pubsub.ValidationReject, result)
	})
}

// blocksByRootProbe connects a peer eligible for parent requests and reports any blocks-by-root stream it receives.
func blocksByRootProbe(t *testing.T, us *p2ptest.TestP2P, clock *startup.Clock) chan struct{} {
	probe := p2ptest.NewTestP2P(t)
	us.Connect(probe)
	us.Peers().Add(new(enr.Record), probe.PeerID(), nil, network.DirOutbound)
	us.Peers().SetConnectionState(probe.PeerID(), peers.Connected)
	us.Peers().SetChainState(probe.PeerID(), &ethpb.StatusV2{FinalizedEpoch: 1, HeadSlot: 1})

	topic, err := p2p.TopicFromMessage(p2p.BeaconBlocksByRootsMessageName, slots.ToEpoch(clock.CurrentSlot()))
	require.NoError(t, err)
	asked := make(chan struct{}, 1)
	probe.BHost.SetStreamHandler(protocol.ID(topic+us.Encoding().ProtocolSuffix()), func(stream network.Stream) {
		select {
		case asked <- struct{}{}:
		default:
		}
		_ = stream.Close()
	})
	return asked
}

// requireNoBlocksByRootRequest fails if the probe peer was asked for blocks within a settle window.
func requireNoBlocksByRootRequest(t *testing.T, asked chan struct{}) {
	select {
	case <-asked:
		t.Fatal("a sidecar with an unknown parent must not trigger a blocks-by-root request")
	case <-time.After(500 * time.Millisecond):
	}
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
