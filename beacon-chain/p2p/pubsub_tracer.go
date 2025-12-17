package p2p

import (
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = pubsub.RawTracer(&gossipTracer{})

// Initializes the values for the pubsub rpc action.
type action int

const (
	recv action = iota
	send
	drop
)

// This tracer is used to implement metrics collection for messages received
// and broadcasted through gossipsub.
type gossipTracer struct {
	host host.Host

	allowedTopics pubsub.SubscriptionFilter

	mu sync.Mutex
	// map topic -> Set(peerID). Peer is in set if it supports partial messages.
	partialMessagePeers map[string]map[peer.ID]struct{}
	// map topic -> Set(peerID). Peer is in set if in the mesh.
	meshPeers map[string]map[peer.ID]struct{}
}

// AddPeer .
func (g *gossipTracer) AddPeer(p peer.ID, proto protocol.ID) {
	// no-op
}

// RemovePeer .
func (g *gossipTracer) RemovePeer(p peer.ID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, peers := range g.partialMessagePeers {
		delete(peers, p)
	}
	for topic, peers := range g.meshPeers {
		if _, ok := peers[p]; ok {
			delete(peers, p)
			g.updateMeshPeersMetric(topic)
		}
	}
}

// Join .
func (g *gossipTracer) Join(topic string) {
	pubsubTopicsActive.WithLabelValues(topic).Set(1)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.partialMessagePeers == nil {
		g.partialMessagePeers = make(map[string]map[peer.ID]struct{})
	}
	if g.partialMessagePeers[topic] == nil {
		g.partialMessagePeers[topic] = make(map[peer.ID]struct{})
	}

	if g.meshPeers == nil {
		g.meshPeers = make(map[string]map[peer.ID]struct{})
	}
	if g.meshPeers[topic] == nil {
		g.meshPeers[topic] = make(map[peer.ID]struct{})
	}
}

// Leave .
func (g *gossipTracer) Leave(topic string) {
	pubsubTopicsActive.WithLabelValues(topic).Set(0)
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.partialMessagePeers, topic)
	delete(g.meshPeers, topic)
}

// Graft .
func (g *gossipTracer) Graft(p peer.ID, topic string) {
	pubsubTopicsGraft.WithLabelValues(topic).Inc()
	g.mu.Lock()
	defer g.mu.Unlock()
	if m, ok := g.meshPeers[topic]; ok {
		m[p] = struct{}{}
	}
	g.updateMeshPeersMetric(topic)
}

// Prune .
func (g *gossipTracer) Prune(p peer.ID, topic string) {
	pubsubTopicsPrune.WithLabelValues(topic).Inc()
	g.mu.Lock()
	defer g.mu.Unlock()
	if m, ok := g.meshPeers[topic]; ok {
		delete(m, p)
	}
	g.updateMeshPeersMetric(topic)
}

// ValidateMessage .
func (g *gossipTracer) ValidateMessage(msg *pubsub.Message) {
	pubsubMessageValidate.WithLabelValues(*msg.Topic).Inc()
}

// DeliverMessage .
func (g *gossipTracer) DeliverMessage(msg *pubsub.Message) {
	pubsubMessageDeliver.WithLabelValues(*msg.Topic).Inc()
}

// RejectMessage .
func (g *gossipTracer) RejectMessage(msg *pubsub.Message, reason string) {
	pubsubMessageReject.WithLabelValues(*msg.Topic, reason).Inc()
}

// DuplicateMessage .
func (g *gossipTracer) DuplicateMessage(msg *pubsub.Message) {
	pubsubMessageDuplicate.WithLabelValues(*msg.Topic).Inc()
}

// UndeliverableMessage .
func (g *gossipTracer) UndeliverableMessage(msg *pubsub.Message) {
	pubsubMessageUndeliverable.WithLabelValues(*msg.Topic).Inc()
}

// ThrottlePeer .
func (g *gossipTracer) ThrottlePeer(p peer.ID) {
	agent := agentFromPid(p, g.host.Peerstore())
	pubsubPeerThrottle.WithLabelValues(agent).Inc()
}

// RecvRPC .
func (g *gossipTracer) RecvRPC(rpc *pubsub.RPC) {
	from := rpc.From()
	g.setMetricFromRPC(recv, pubsubRPCSubRecv, pubsubRPCPubRecv, pubsubRPCPubRecvSize, pubsubRPCRecv, rpc)

	g.mu.Lock()
	defer g.mu.Unlock()
	for _, sub := range rpc.Subscriptions {
		topic := sub.GetTopicid()
		if !g.allowedTopics.CanSubscribe(topic) {
			continue
		}
		if g.partialMessagePeers == nil {
			g.partialMessagePeers = make(map[string]map[peer.ID]struct{})
		}
		m, ok := g.partialMessagePeers[topic]
		if !ok {
			m = make(map[peer.ID]struct{})
			g.partialMessagePeers[topic] = m
		}
		if sub.GetSubscribe() && sub.GetRequestsPartial() {
			m[from] = struct{}{}
		} else {
			delete(m, from)
			if len(m) == 0 {
				delete(g.partialMessagePeers, topic)
			}
		}
	}
}

// SendRPC .
func (g *gossipTracer) SendRPC(rpc *pubsub.RPC, p peer.ID) {
	g.setMetricFromRPC(send, pubsubRPCSubSent, pubsubRPCPubSent, pubsubRPCPubSentSize, pubsubRPCSent, rpc)
}

// DropRPC .
func (g *gossipTracer) DropRPC(rpc *pubsub.RPC, p peer.ID) {
	g.setMetricFromRPC(drop, pubsubRPCSubDrop, pubsubRPCPubDrop, pubsubRPCPubDropSize, pubsubRPCDrop, rpc)
}

func (g *gossipTracer) setMetricFromRPC(act action, subCtr prometheus.Counter, pubCtr, pubSizeCtr, ctrlCtr *prometheus.CounterVec, rpc *pubsub.RPC) {
	subCtr.Add(float64(len(rpc.Subscriptions)))
	if rpc.Control != nil {
		ctrlCtr.WithLabelValues("graft").Add(float64(len(rpc.Control.Graft)))
		ctrlCtr.WithLabelValues("prune").Add(float64(len(rpc.Control.Prune)))
		ctrlCtr.WithLabelValues("ihave").Add(float64(len(rpc.Control.Ihave)))
		ctrlCtr.WithLabelValues("iwant").Add(float64(len(rpc.Control.Iwant)))
		ctrlCtr.WithLabelValues("idontwant").Add(float64(len(rpc.Control.Idontwant)))
	}
	// For incoming messages from pubsub, we do not record metrics for them as these values
	// could be junk.
	if act == recv {
		return
	}
	for _, msg := range rpc.Publish {
		pubCtr.WithLabelValues(msg.GetTopic()).Inc()
		pubSizeCtr.WithLabelValues(msg.GetTopic(), "false").Add(float64(msg.Size()))
	}
	if rpc.Partial != nil {
		pubCtr.WithLabelValues(rpc.Partial.GetTopicID()).Inc()
		pubSizeCtr.WithLabelValues(rpc.Partial.GetTopicID(), "true").Add(float64(rpc.Partial.Size()))
	}
}

// updateMeshPeersMetric requires the caller to hold the state mutex
func (g *gossipTracer) updateMeshPeersMetric(topic string) {
	meshPeers, ok := g.meshPeers[topic]
	if !ok {
		return
	}
	partialPeers, ok := g.partialMessagePeers[topic]
	if !ok {
		return
	}

	var supportsPartial, doesNotSupportPartial float64
	for p := range meshPeers {
		if _, ok := partialPeers[p]; ok {
			supportsPartial++
		} else {
			doesNotSupportPartial++
		}
	}

	pubsubMeshPeers.WithLabelValues(topic, "true").Set(supportsPartial)
	pubsubMeshPeers.WithLabelValues(topic, "false").Set(doesNotSupportPartial)
}
