// Package partialmsgmux multiplexes the single gossipsub partial-messages
// extension between the data column and attestation topic families. The
// pubsub library supports exactly one extension instance per host with one
// concrete peer-state type, so the mux registers the extension with the union
// blocks.PartialMessagePeerState and routes each callback to the family whose
// topic matches. Each family owns one field of the union and must not touch
// the others.
package partialmsgmux

import (
	"log/slog"
	"strings"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/internal/logrusadapter"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("package", "beacon-chain/p2p/partialmsgmux")

const (
	dataColumnTopicSubstring  = "data_column_sidecar_"
	attestationTopicSubstring = "beacon_attestation"
)

// PeerFeedbackFn reports message quality feedback for a peer to gossipsub scoring.
type PeerFeedbackFn func(topic string, p peer.ID, kind pubsub.PeerFeedbackKind) error

// PublishPartialFn publishes partial-message actions for one (topic, group).
type PublishPartialFn func(topic string, groupID []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error

// Handler consumes partial-message events for one topic family. Both
// callbacks run on the gossipsub event loop and must be fast and
// non-blocking. InitPubSub is called once at pubsub construction with the
// hooks the handler needs to report feedback and publish partial messages.
type Handler interface {
	OnIncomingRPC(from peer.ID, peerStates map[peer.ID]blocks.PartialMessagePeerState, rpc *pubsub_pb.PartialMessagesExtension) error
	OnEmitGossip(topic string, groupID []byte, gossipPeers []peer.ID, peerStates map[peer.ID]blocks.PartialMessagePeerState)
	InitPubSub(peerFeedback PeerFeedbackFn, publishPartial PublishPartialFn)
}

// Mux routes partial-message extension callbacks by topic.
type Mux struct {
	dataColumns Handler
	att         Handler
}

// New creates an empty mux.
func New() *Mux {
	return &Mux{}
}

// RegisterDataColumnHandler routes data column topic callbacks to h.
func (m *Mux) RegisterDataColumnHandler(h Handler) {
	m.dataColumns = h
}

// RegisterAttestationHandler routes attestation subnet topic callbacks to h.
func (m *Mux) RegisterAttestationHandler(h Handler) {
	m.att = h
}

// handlerFor returns the family handler for the topic, nil if none.
func (m *Mux) handlerFor(topic string) Handler {
	switch {
	case m.dataColumns != nil && strings.Contains(topic, dataColumnTopicSubstring):
		return m.dataColumns
	case m.att != nil && strings.Contains(topic, attestationTopicSubstring):
		return m.att
	}
	return nil
}

// onIncomingRPC dispatches an extension RPC; the extension guarantees rpc is
// non-nil. The topic is peer-controlled; an unknown topic is not penalized.
func (m *Mux) onIncomingRPC(from peer.ID, peerStates map[peer.ID]blocks.PartialMessagePeerState, rpc *pubsub_pb.PartialMessagesExtension) error {
	h := m.handlerFor(rpc.GetTopicID())
	if h == nil {
		log.WithFields(logrus.Fields{"peer": from, "topic": rpc.GetTopicID()}).
			Debug("No partial message handler for topic")
		return nil
	}
	return h.OnIncomingRPC(from, peerStates, rpc)
}

func (m *Mux) onEmitGossip(topic string, groupID []byte, gossipPeers []peer.ID, peerStates map[peer.ID]blocks.PartialMessagePeerState) {
	if h := m.handlerFor(topic); h != nil {
		h.OnEmitGossip(topic, groupID, gossipPeers, peerStates)
	}
}

// AppendPubSubOpts appends the options that install the partial-messages
// extension and hand each registered handler its pubsub hooks.
func (m *Mux) AppendPubSubOpts(opts []pubsub.Option) []pubsub.Option {
	slogger := slog.New(logrusadapter.Handler{Logger: log.Logger}).With("package", "partialmsgmux")
	return append(opts,
		pubsub.WithPartialMessagesExtension(&partialmessages.PartialMessagesExtension[blocks.PartialMessagePeerState]{
			Logger:        slogger,
			OnEmitGossip:  m.onEmitGossip,
			OnIncomingRPC: m.onIncomingRPC,
		}),
		func(ps *pubsub.PubSub) error {
			publishPartial := func(topic string, groupID []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error {
				return pubsub.PublishPartial(ps, topic, groupID, fn)
			}
			if m.dataColumns != nil {
				m.dataColumns.InitPubSub(ps.PeerFeedback, publishPartial)
			}
			if m.att != nil {
				m.att.InitPubSub(ps.PeerFeedback, publishPartial)
			}
			return nil
		},
	)
}
