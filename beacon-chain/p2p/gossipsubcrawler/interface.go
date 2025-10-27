package gossipsubcrawler

import (
	"context"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TopicExtractor is a function that can determine the set of topics a current or potential peer
// is subscribed to based on key/value pairs from the ENR record.
type TopicExtractor func(ctx context.Context, node *enode.Node) ([]string, error)

// Crawler is a dependency-light interface
// for the gossip peer crawler used by sync. Keeping this in an inner
// package avoids circular imports between p2p and its testing helpers.
type Crawler interface {
	Start(topicExtractor TopicExtractor) error
	Stop()
	RemoveTopic(topic string)
	RemovePeer(enodeID enode.ID)
	RemovePeerByPeerID(pid peer.ID)
	PeersForTopic(topic string) []*enode.Node
}
