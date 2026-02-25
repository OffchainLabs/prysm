package testing

import (
	"context"
	"errors"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

type topicPeer struct {
	topic string
	peer  peer.ID
}

// GossipTracer implements pubsub.RawTracer for use in tests. It allows callers
// to block until specific gossipsub-internal events have fired, which is useful
// for avoiding races between the various maps maintained by the pubsub event loop.
//
// Individual methods (RemovePeer, Prune, ValidateMessage, etc.) can be extended
// as needed by future tests.
type GossipTracer struct {
	mu       sync.Mutex
	peers    map[peer.ID]chan struct{}
	addedSet map[peer.ID]struct{}

	joinedTopics map[string]struct{}

	graftSet     map[topicPeer]struct{}
	grafts       map[topicPeer]chan struct{}
	topicWaiters map[string]*TopicEventWaiter
}

// NewGossipTracer returns a new tracer ready for use. Pass it to
// pubsub.NewGossipSub via pubsub.WithRawTracer(tracer).
func NewGossipTracer() *GossipTracer {
	return &GossipTracer{
		peers:        make(map[peer.ID]chan struct{}),
		addedSet:     make(map[peer.ID]struct{}),
		joinedTopics: make(map[string]struct{}),
		graftSet:     make(map[topicPeer]struct{}),
		grafts:       make(map[topicPeer]chan struct{}),
		topicWaiters: make(map[string]*TopicEventWaiter),
	}
}

func (t *GossipTracer) waitForAddPeer(ctx context.Context, pid peer.ID) error {
	t.mu.Lock()
	if _, ok := t.addedSet[pid]; ok {
		t.mu.Unlock()
		return nil
	}
	ch, ok := t.peers[pid]
	if !ok {
		ch = make(chan struct{})
		t.peers[pid] = ch
	}
	t.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *GossipTracer) waitForGraft(ctx context.Context, topic string, pid peer.ID) error {
	key := topicPeer{topic: topic, peer: pid}

	t.mu.Lock()
	if _, ok := t.graftSet[key]; ok {
		t.mu.Unlock()
		return nil
	}
	ch, ok := t.grafts[key]
	if !ok {
		ch = make(chan struct{})
		t.grafts[key] = ch
	}
	t.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *GossipTracer) isSubscribed(topic string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.joinedTopics[topic]
	return ok
}

// CanPublishToPeer blocks until the gossipsub event loop is in a state where
// publishing a message on the given topic will successfully reach pid.
//
// The conditions depend on whether we have locally subscribed to the topic:
//   - Subscribed (mesh path): waits until pid has been grafted into our mesh
//     for the topic.
//   - Not subscribed (fanout path): waits until both PeerJoin (pid is in
//     p.topics[topic]) and AddPeer (pid is in p.peers with an rpcQueue) have
//     fired.
//
// A TopicEventWaiter must have been set up for the topic via WatchTopic before
// calling this method when not subscribed, otherwise PeerJoin cannot be observed.
// Note: You must call WatchTopic first before calling this method.
func (t *GossipTracer) CanPublishToPeer(ctx context.Context, topic string, pid peer.ID) error {
	if t.isSubscribed(topic) {
		return t.waitForGraft(ctx, topic, pid)
	}

	// Fanout path: need both PeerJoin and AddPeer.
	w := t.getTopicWaiter(topic)
	if w == nil {
		return errors.New("topic waiter not found, please call WatchTopic first")
	}
	if err := w.waitForPeerJoin(ctx, pid); err != nil {
		return err
	}
	return t.waitForAddPeer(ctx, pid)
}

// TopicEventWaiter tracks PeerJoin/PeerLeave events for a single topic.
type TopicEventWaiter struct {
	mu      sync.Mutex
	joined  map[peer.ID]struct{}
	waiters map[peer.ID]chan struct{}
}

// WatchTopic creates a TopicEventHandler on the given pubsub.Topic and starts
// a background goroutine that drains peer events sent to the TopicEventHandler by gossipsub .
func (t *GossipTracer) WatchTopic(ctx context.Context, topicHandle *pubsub.Topic) error {
	ev, err := topicHandle.EventHandler()
	if err != nil {
		return err
	}

	w := &TopicEventWaiter{
		joined:  make(map[peer.ID]struct{}),
		waiters: make(map[peer.ID]chan struct{}),
	}

	// Register the waiter so CanPublishToPeer can find it.
	t.mu.Lock()
	t.topicWaiters[topicHandle.String()] = w
	t.mu.Unlock()

	go func() {
		defer ev.Cancel()
		for {
			if ctx.Err() != nil {
				return
			}
			pe, err := ev.NextPeerEvent(ctx)
			if err != nil {
				return
			}
			if pe.Type == pubsub.PeerJoin {
				w.handlePeerJoin(pe.Peer)
			}
		}
	}()

	return nil
}

func (t *GossipTracer) getTopicWaiter(topic string) *TopicEventWaiter {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.topicWaiters[topic]
}

func (w *TopicEventWaiter) handlePeerJoin(pid peer.ID) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.joined[pid] = struct{}{}
	if ch, ok := w.waiters[pid]; ok {
		close(ch)
		delete(w.waiters, pid)
	}
}

func (w *TopicEventWaiter) waitForPeerJoin(ctx context.Context, pid peer.ID) error {
	w.mu.Lock()
	if _, ok := w.joined[pid]; ok {
		w.mu.Unlock()
		return nil
	}
	ch, ok := w.waiters[pid]
	if !ok {
		ch = make(chan struct{})
		w.waiters[pid] = ch
	}
	w.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- pubsub.RawTracer implementation ---

// AddPeer is invoked by the gossipsub event loop after a peer has been fully
// registered in p.peers (i.e., it has an rpcQueue and an outbound stream).
func (t *GossipTracer) AddPeer(p peer.ID, proto protocol.ID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.addedSet[p] = struct{}{}
	if ch, ok := t.peers[p]; ok {
		close(ch)
		delete(t.peers, p)
	}
}

// RemovePeer can be extended by future tests to track peer removal.
func (t *GossipTracer) RemovePeer(p peer.ID) {}

// Join is invoked when we locally subscribe to a topic (a mesh is created).
func (t *GossipTracer) Join(topic string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.joinedTopics[topic] = struct{}{}
}

// Leave is invoked when we unsubscribe from a topic (mesh is torn down).
func (t *GossipTracer) Leave(topic string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.joinedTopics, topic)
}

// Graft is invoked when a peer is added to our mesh for a topic.
func (t *GossipTracer) Graft(p peer.ID, topic string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := topicPeer{topic: topic, peer: p}
	t.graftSet[key] = struct{}{}
	if ch, ok := t.grafts[key]; ok {
		close(ch)
		delete(t.grafts, key)
	}
}

// Prune can be extended by future tests to track mesh prunes.
func (t *GossipTracer) Prune(p peer.ID, topic string) {}

// ValidateMessage can be extended by future tests to track message validation.
func (t *GossipTracer) ValidateMessage(msg *pubsub.Message) {}

// DeliverMessage can be extended by future tests to track message delivery.
func (t *GossipTracer) DeliverMessage(msg *pubsub.Message) {}

// RejectMessage can be extended by future tests to track message rejection.
func (t *GossipTracer) RejectMessage(msg *pubsub.Message, reason string) {}

// DuplicateMessage can be extended by future tests to track duplicate messages.
func (t *GossipTracer) DuplicateMessage(msg *pubsub.Message) {}

// ThrottlePeer can be extended by future tests to track peer throttling.
func (t *GossipTracer) ThrottlePeer(p peer.ID) {}

// RecvRPC can be extended by future tests to track incoming RPCs.
func (t *GossipTracer) RecvRPC(rpc *pubsub.RPC) {}

// SendRPC can be extended by future tests to track outgoing RPCs.
func (t *GossipTracer) SendRPC(rpc *pubsub.RPC, p peer.ID) {}

// DropRPC can be extended by future tests to track dropped RPCs.
func (t *GossipTracer) DropRPC(rpc *pubsub.RPC, p peer.ID) {}

// UndeliverableMessage can be extended by future tests to track undeliverable messages.
func (t *GossipTracer) UndeliverableMessage(msg *pubsub.Message) {}
