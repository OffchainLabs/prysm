package sync

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

func TestValidateExecutionProof(t *testing.T) {
	ctx := context.Background()
	blockRoot := bytesutil.ToBytes32([]byte("block"))

	envelope := func(proofType ethpb.ProofType, proofData []byte, prover primitives.ValidatorIndex) *ethpb.SignedExecutionProofEnvelope {
		return &ethpb.SignedExecutionProofEnvelope{
			Message: &ethpb.ExecutionProofEnvelope{
				ProofData:       proofData,
				ProofType:       []byte{byte(proofType)},
				BeaconBlockRoot: blockRoot[:],
			},
			ValidatorIndex: prover,
			Signature:      make([]byte, 96),
		}
	}

	// newService returns a synced service whose chain knows the beacon block.
	newService := func(t *testing.T, chain *mockChain.ChainService) *Service {
		chain.DB = dbtest.SetupDB(t)
		if chain.InitSyncBlockRoots == nil {
			chain.InitSyncBlockRoots = map[[32]byte]bool{blockRoot: true}
		}

		s := &Service{cfg: &config{
			p2p:         p2ptest.NewTestP2P(t),
			initialSync: &mockSync.Sync{},
			chain:       chain,
			clock:       startup.NewClock(time.Now(), [32]byte{}),
		}}
		s.initCaches()
		return s
	}

	toPubsub := func(t *testing.T, s *Service, e *ethpb.SignedExecutionProofEnvelope) *pubsub.Message {
		buf := new(bytes.Buffer)
		_, err := s.cfg.p2p.Encoding().EncodeGossip(buf, e)
		require.NoError(t, err)

		topic := p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.SignedExecutionProofEnvelope]()]
		topic = s.addDigestToTopic(topic, s.currentForkDigest())

		return &pubsub.Message{Message: &pb.Message{Data: buf.Bytes(), Topic: &topic}}
	}

	validate := func(t *testing.T, s *Service, e *ethpb.SignedExecutionProofEnvelope) pubsub.ValidationResult {
		result, _ := s.validateExecutionProof(ctx, "peer", toPubsub(t, s, e))
		return result
	}

	t.Run("valid", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{})
		msg := toPubsub(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1))

		result, err := s.validateExecutionProof(ctx, "peer", msg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
		require.NotNil(t, msg.ValidatorData)
	})

	t.Run("syncing", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{})
		s.cfg.initialSync = &mockSync.Sync{IsSyncing: true}
		require.Equal(t, pubsub.ValidationIgnore, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))
	})

	t.Run("empty proof data", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{})
		require.Equal(t, pubsub.ValidationReject, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, nil, 1)))
	})

	t.Run("unsupported proof type", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{})
		require.Equal(t, pubsub.ValidationReject, validate(t, s, envelope(ethpb.ProofType(0), []byte{0x01}, 1)))
	})

	t.Run("unknown beacon block", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{InitSyncBlockRoots: map[[32]byte]bool{}})
		require.Equal(t, pubsub.ValidationIgnore, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))
	})

	t.Run("proof type already verified", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{
			ExecutionProofTypes: map[[32]byte]map[ethpb.ProofType]bool{blockRoot: {ethpb.ProofTypeEthrexSP1: true}},
		})
		require.Equal(t, pubsub.ValidationIgnore, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))
	})

	t.Run("proof already processed", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{})
		require.Equal(t, pubsub.ValidationAccept, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))
		require.Equal(t, pubsub.ValidationIgnore, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))
	})

	t.Run("prover already seen for this block and proof type", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{})
		require.Equal(t, pubsub.ValidationAccept, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))
		require.Equal(t, pubsub.ValidationIgnore, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x02}, 1)))
		require.Equal(t, pubsub.ValidationAccept, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x02}, 2)))
	})

	t.Run("payload unavailable", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{CheckExecutionProofErr: blockchain.ErrExecutionProofPayloadUnavailable})
		require.Equal(t, pubsub.ValidationIgnore, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))
	})

	t.Run("invalid envelope", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{CheckExecutionProofErr: blockchain.ErrInvalidExecutionProofEnvelope})
		require.Equal(t, pubsub.ValidationReject, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))
	})

	t.Run("invalid proof", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{VerifyExecutionProofErr: blockchain.ErrInvalidExecutionProof})
		require.Equal(t, pubsub.ValidationReject, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))

		// The prover attempt is marked as seen before the proof is verified.
		require.Equal(t, pubsub.ValidationIgnore, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x02}, 1)))
	})

	t.Run("verification failure", func(t *testing.T) {
		s := newService(t, &mockChain.ChainService{VerifyExecutionProofErr: errors.New("no verifier")})
		require.Equal(t, pubsub.ValidationIgnore, validate(t, s, envelope(ethpb.ProofTypeEthrexSP1, []byte{0x01}, 1)))
	})
}
