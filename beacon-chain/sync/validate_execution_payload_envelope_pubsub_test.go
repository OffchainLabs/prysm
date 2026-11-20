package sync

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

// countingRealEnvelopeVerifier records production BLS signature checks while
// retaining every production envelope verification method.
type countingRealEnvelopeVerifier struct {
	verification.ExecutionPayloadEnvelopeVerifier
	calls *atomic.Int32
}

func (v *countingRealEnvelopeVerifier) VerifySignatureWithPubkey(pubkey [fieldparams.BLSPubkeyLength]byte, genesisRoot [32]byte) error {
	v.calls.Add(1)
	return v.ExecutionPayloadEnvelopeVerifier.VerifySignatureWithPubkey(pubkey, genesisRoot)
}

type envelopeDuplicateTracer struct {
	*p2ptest.GossipTracer
	duplicates chan *pubsub.Message
}

func (t *envelopeDuplicateTracer) DuplicateMessage(msg *pubsub.Message) {
	select {
	case t.duplicates <- msg:
	default:
	}
}

type envelopeValidation struct {
	from   peer.ID
	result pubsub.ValidationResult
	err    error
}

func TestExecutionPayloadEnvelopePubsub_InvalidPeerCannotSuppressValidEnvelope(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	s, original, builderIdx, root := setupExecutionPayloadEnvelopeService(t, 1, 1)
	s.badBlockCache = lruwrpr.New(10)
	oldClock := s.cfg.clock
	s.cfg.clock = startup.NewClock(oldClock.GenesisTime(), oldClock.GenesisValidatorsRoot(), startup.WithSlotAsNow(1))
	oldReceiver := s.cfg.p2p.(*p2ptest.TestP2P)
	t.Cleanup(func() { require.NoError(t, oldReceiver.BHost.Close()) })
	receiverTracer := &envelopeDuplicateTracer{
		GossipTracer: p2ptest.NewGossipTracer(),
		duplicates:   make(chan *pubsub.Message, 8),
	}
	genesisRoot := s.cfg.clock.GenesisValidatorsRoot()
	msgIDOption := pubsub.WithMessageIdFn(func(msg *pubsubpb.Message) string {
		return p2p.MsgID(genesisRoot[:], msg)
	})
	receiver := p2ptest.NewTestP2PWithPubsubOptions(t, []pubsub.Option{msgIDOption, pubsub.WithRawTracer(receiverTracer)})
	peerATracer := p2ptest.NewGossipTracer()
	peerA := p2ptest.NewTestP2PWithPubsubOptions(t, []pubsub.Option{msgIDOption, pubsub.WithRawTracer(peerATracer)})
	peerBTracer := p2ptest.NewGossipTracer()
	peerB := p2ptest.NewTestP2PWithPubsubOptions(t, []pubsub.Option{msgIDOption, pubsub.WithRawTracer(peerBTracer)})
	for _, p := range []*p2ptest.TestP2P{receiver, peerA, peerB} {
		p := p
		t.Cleanup(func() { require.NoError(t, p.BHost.Close()) })
	}
	s.cfg.p2p = receiver

	var signatureCalls atomic.Int32
	s.newExecutionPayloadEnvelopeVerifier = func(e interfaces.ROSignedExecutionPayloadEnvelope, reqs []verification.Requirement) verification.ExecutionPayloadEnvelopeVerifier {
		return &countingRealEnvelopeVerifier{
			ExecutionPayloadEnvelopeVerifier: verification.NewEnvelopeVerifier(e, reqs),
			calls:                            &signatureCalls,
		}
	}

	decoded, err := s.decodePubsubMessage(original)
	require.NoError(t, err)
	valid := decoded.(*ethpb.SignedExecutionPayloadEnvelope)
	builderKey, err := bls.RandKey()
	require.NoError(t, err)
	wrongKey1, err := bls.RandKey()
	require.NoError(t, err)
	wrongKey2, err := bls.RandKey()
	require.NoError(t, err)
	var builderPubkey [fieldparams.BLSPubkeyLength]byte
	copy(builderPubkey[:], builderKey.PublicKey().Marshal())
	s.cfg.chain.(*mock.ChainService).BuilderPubkeys[root] = &builderPubkey
	fork, err := params.Fork(slots.ToEpoch(1))
	require.NoError(t, err)
	domain, err := signing.Domain(fork, slots.ToEpoch(1), params.BeaconConfig().DomainBeaconBuilder, genesisRoot[:])
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(valid.Message, domain)
	require.NoError(t, err)
	valid.Signature = builderKey.Sign(signingRoot[:]).Marshal()
	invalid1 := proto.Clone(valid).(*ethpb.SignedExecutionPayloadEnvelope)
	invalid1.Signature = wrongKey1.Sign(signingRoot[:]).Marshal()
	invalid2 := proto.Clone(valid).(*ethpb.SignedExecutionPayloadEnvelope)
	invalid2.Signature = wrongKey2.Sign(signingRoot[:]).Marshal()
	bad1Msg := envelopeToPubsub(t, s, receiver, invalid1)
	bad2Msg := envelopeToPubsub(t, s, receiver, invalid2)
	validMsg := envelopeToPubsub(t, s, receiver, valid)
	messageID := func(msg *pubsub.Message) string { return p2p.MsgID(genesisRoot[:], msg.Message) }
	require.NotEqual(t, messageID(bad1Msg), messageID(bad2Msg))
	require.NotEqual(t, messageID(bad1Msg), messageID(validMsg))
	require.NotEqual(t, messageID(bad2Msg), messageID(validMsg))
	topic := *validMsg.Topic

	validations := make(chan envelopeValidation, 4)
	require.NoError(t, receiver.PubSub().RegisterTopicValidator(topic,
		func(ctx context.Context, from peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
			result, err := s.validateExecutionPayloadEnvelope(ctx, from, msg)
			validations <- envelopeValidation{from: from, result: result, err: err}
			return result
		}))
	_, err = receiver.SubscribeToTopic(topic)
	require.NoError(t, err)
	_, err = peerATracer.JoinAndWatchTopic(ctx, topic, peerA)
	require.NoError(t, err)
	_, err = peerBTracer.JoinAndWatchTopic(ctx, topic, peerB)
	require.NoError(t, err)
	peerA.Connect(receiver)
	peerB.Connect(receiver)
	require.NoError(t, peerATracer.CanPublishToPeer(ctx, topic, receiver.PeerID()))
	require.NoError(t, peerBTracer.CanPublishToPeer(ctx, topic, receiver.PeerID()))

	awaitValidation := func(from peer.ID) envelopeValidation {
		t.Helper()
		select {
		case got := <-validations:
			require.Equal(t, from, got.from)
			return got
		case <-ctx.Done():
			t.Fatalf("timed out waiting for validation from %s: %v", from, ctx.Err())
			return envelopeValidation{}
		}
	}
	for _, msg := range []*pubsub.Message{bad1Msg, bad2Msg} {
		require.NoError(t, peerA.PublishToTopic(ctx, topic, msg.Data))
		got := awaitValidation(peerA.PeerID())
		require.Equal(t, pubsub.ValidationReject, got.result)
		require.ErrorIs(t, got.err, signing.ErrSigFailedToVerify)
	}
	require.Equal(t, int32(2), signatureCalls.Load())
	require.NoError(t, peerA.PublishToTopic(ctx, topic, validMsg.Data))
	fromA := awaitValidation(peerA.PeerID())

	// At least one copy of the valid envelope must pass gossip validation. Gossipsub drops B's copy
	// as a duplicate once A's copy has been validated, so A's earlier invalid signatures must not
	// cause its own copy to be ignored.
	require.NoError(t, peerB.PublishToTopic(ctx, topic, bytes.Clone(validMsg.Data)))
	if fromA.result == pubsub.ValidationAccept {
		require.NoError(t, fromA.err)
		require.Equal(t, true, s.hasSeenPayloadEnvelope(root, builderIdx))
		return
	}
	select {
	case got := <-validations:
		require.Equal(t, peerB.PeerID(), got.from)
		require.Equal(t, pubsub.ValidationAccept, got.result, "validation error: %v", got.err)
		require.NoError(t, got.err)
	case duplicate := <-receiverTracer.duplicates:
		require.Equal(t, peerB.PeerID(), duplicate.ReceivedFrom)
		require.Equal(t, messageID(validMsg), messageID(duplicate))
		t.Fatalf("peer B's valid envelope was dropped as a duplicate after A's copy returned %v: %v", fromA.result, fromA.err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for peer B's valid envelope: %v", ctx.Err())
	}
	require.Equal(t, true, s.hasSeenPayloadEnvelope(root, builderIdx))
}
