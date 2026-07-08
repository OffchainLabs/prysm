package partialmsgmux

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
)

type recordingHandler struct {
	incomingTopics []string
	gossipTopics   []string
}

func (h *recordingHandler) OnIncomingRPC(_ peer.ID, _ map[peer.ID]blocks.PartialMessagePeerState, rpc *pubsub_pb.PartialMessagesExtension) error {
	h.incomingTopics = append(h.incomingTopics, rpc.GetTopicID())
	return nil
}

func (h *recordingHandler) OnEmitGossip(topic string, _ []byte, _ []peer.ID, _ map[peer.ID]blocks.PartialMessagePeerState) {
	h.gossipTopics = append(h.gossipTopics, topic)
}

func (*recordingHandler) InitPubSub(PeerFeedbackFn, PublishPartialFn) {}

func rpcForTopic(topic string) *pubsub_pb.PartialMessagesExtension {
	return &pubsub_pb.PartialMessagesExtension{TopicID: &topic}
}

func TestMuxRoutesByTopic(t *testing.T) {
	columns := &recordingHandler{}
	atts := &recordingHandler{}
	m := New()
	m.RegisterDataColumnHandler(columns)
	m.RegisterAttestationHandler(atts)

	attTopic := "/eth2/aabbccdd/beacon_attestation_5/ssz_snappy"
	colTopic := "/eth2/aabbccdd/data_column_sidecar_3/ssz_snappy"

	require.NoError(t, m.onIncomingRPC("peer", nil, rpcForTopic(attTopic)))
	require.NoError(t, m.onIncomingRPC("peer", nil, rpcForTopic(colTopic)))
	// Unknown topics are dropped without error: the topic string is
	// peer-controlled, so it must not be able to trigger a failure.
	require.NoError(t, m.onIncomingRPC("peer", nil, rpcForTopic("/eth2/aabbccdd/beacon_block/ssz_snappy")))

	require.DeepEqual(t, []string{attTopic}, atts.incomingTopics)
	require.DeepEqual(t, []string{colTopic}, columns.incomingTopics)

	m.onEmitGossip(attTopic, nil, nil, nil)
	m.onEmitGossip("/eth2/aabbccdd/beacon_block/ssz_snappy", nil, nil, nil)
	require.DeepEqual(t, []string{attTopic}, atts.gossipTopics)
	require.Equal(t, 0, len(columns.gossipTopics))
}
