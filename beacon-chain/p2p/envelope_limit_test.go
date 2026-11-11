package p2p

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
)

var testEnvelopeTopic = fmt.Sprintf(ExecutionPayloadEnvelopeTopicFormat, [4]byte{}) + encoder.SszNetworkEncoder{}.ProtocolSuffix()

func TestEnvelopeLimiter_Admit(t *testing.T) {
	l := &envelopeLimiter{}
	// Nothing is counted until genesis is known.
	require.Equal(t, 100, l.admit("a", 100))

	l.setGenesis(time.Now().Add(-params.BeaconConfig().SlotDuration()))
	require.Equal(t, maxEnvelopesPerPeerPerSlot-1, l.admit("a", maxEnvelopesPerPeerPerSlot-1))
	require.Equal(t, 1, l.admit("a", 2))
	require.Equal(t, 0, l.admit("a", 1))
	require.Equal(t, maxEnvelopesPerPeerPerSlot, l.admit("b", maxEnvelopesPerPeerPerSlot+1))

	// The allowance resets in the next slot.
	l.setGenesis(time.Now().Add(-2 * params.BeaconConfig().SlotDuration()))
	require.Equal(t, maxEnvelopesPerPeerPerSlot, l.admit("a", maxEnvelopesPerPeerPerSlot+1))
}

func TestService_InspectRPC_DropsEnvelopesOverLimit(t *testing.T) {
	s := &Service{}
	s.envelopeLimiter.setGenesis(time.Now())
	bidTopic := fmt.Sprintf(ExecutionPayloadBidTopicFormat, [4]byte{}) + encoder.SszNetworkEncoder{}.ProtocolSuffix()
	newRPC := func(topics ...string) *pubsub.RPC {
		rpc := &pubsub.RPC{}
		for i, topic := range topics {
			rpc.Publish = append(rpc.Publish, &pubsubpb.Message{Topic: &topic, Data: []byte{byte(i)}})
		}
		return rpc
	}

	for range maxEnvelopesPerPeerPerSlot - 1 {
		rpc := newRPC(testEnvelopeTopic)
		require.NoError(t, s.inspectRPC("a", rpc))
		require.Equal(t, 1, len(rpc.Publish))
	}
	rpc := newRPC(testEnvelopeTopic, bidTopic, testEnvelopeTopic, testEnvelopeTopic)
	require.NoError(t, s.inspectRPC("a", rpc))
	require.Equal(t, 2, len(rpc.Publish))
	require.Equal(t, testEnvelopeTopic, rpc.Publish[0].GetTopic())
	require.Equal(t, bidTopic, rpc.Publish[1].GetTopic())

	// Other peers keep their own allowance.
	rpc = newRPC(testEnvelopeTopic)
	require.NoError(t, s.inspectRPC("b", rpc))
	require.Equal(t, 1, len(rpc.Publish))
}

// A dropped envelope must not be marked as seen, otherwise gossipsub would discard every other
// peer's copy of it. This also pins that gossipsub processes the RPC as edited by the inspector.
func TestService_InspectRPC_DroppedEnvelopeIsNotMarkedSeen(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	msgID := pubsub.WithMessageIdFn(func(msg *pubsubpb.Message) string { return string(msg.Data) })
	tracerA, tracerB := p2ptest.NewGossipTracer(), p2ptest.NewGossipTracer()
	peerA := p2ptest.NewTestP2PWithPubsubOptions(t, []pubsub.Option{msgID, pubsub.WithRawTracer(tracerA)})
	peerB := p2ptest.NewTestP2PWithPubsubOptions(t, []pubsub.Option{msgID, pubsub.WithRawTracer(tracerB)})

	shared := []byte("shared")
	hasShared := func(rpc *pubsub.RPC) bool {
		for _, msg := range rpc.Publish {
			if string(msg.Data) == string(shared) {
				return true
			}
		}
		return false
	}
	s := &Service{}
	s.envelopeLimiter.setGenesis(time.Now())
	droppedFromA := make(chan struct{}, 1)
	receiver := p2ptest.NewTestP2PWithPubsubOptions(t, []pubsub.Option{msgID, pubsub.WithAppSpecificRpcInspector(
		func(from peer.ID, rpc *pubsub.RPC) error {
			hadShared := hasShared(rpc)
			err := s.inspectRPC(from, rpc)
			if from == peerA.PeerID() && hadShared && !hasShared(rpc) {
				droppedFromA <- struct{}{}
			}
			return err
		})})
	for _, p := range []*p2ptest.TestP2P{receiver, peerA, peerB} {
		t.Cleanup(func() { require.NoError(t, p.BHost.Close()) })
	}

	validated := make(chan peer.ID, 2*maxEnvelopesPerPeerPerSlot)
	require.NoError(t, receiver.PubSub().RegisterTopicValidator(testEnvelopeTopic,
		func(_ context.Context, from peer.ID, _ *pubsub.Message) pubsub.ValidationResult {
			validated <- from
			return pubsub.ValidationAccept
		}))
	_, err := receiver.SubscribeToTopic(testEnvelopeTopic)
	require.NoError(t, err)
	_, err = tracerA.JoinAndWatchTopic(ctx, testEnvelopeTopic, peerA)
	require.NoError(t, err)
	_, err = tracerB.JoinAndWatchTopic(ctx, testEnvelopeTopic, peerB)
	require.NoError(t, err)
	peerA.Connect(receiver)
	peerB.Connect(receiver)
	require.NoError(t, tracerA.CanPublishToPeer(ctx, testEnvelopeTopic, receiver.PeerID()))
	require.NoError(t, tracerB.CanPublishToPeer(ctx, testEnvelopeTopic, receiver.PeerID()))

	awaitValidation := func(want peer.ID) {
		t.Helper()
		select {
		case got := <-validated:
			require.Equal(t, want, got)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for a validation from %s", want)
		}
	}
	for i := range maxEnvelopesPerPeerPerSlot {
		require.NoError(t, peerA.PublishToTopic(ctx, testEnvelopeTopic, []byte{byte(i)}))
		awaitValidation(peerA.PeerID())
	}
	require.NoError(t, peerA.PublishToTopic(ctx, testEnvelopeTopic, shared))
	select {
	case <-droppedFromA:
	case <-ctx.Done():
		t.Fatal("timed out waiting for peer A's envelope over the limit to be dropped")
	}

	require.NoError(t, peerB.PublishToTopic(ctx, testEnvelopeTopic, shared))
	awaitValidation(peerB.PeerID())
	require.Equal(t, 0, len(validated))
}
