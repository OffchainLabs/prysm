package gossipsubcrawler

import (
	"context"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
)

type Topic string

// TopicExtractor is a function that can determine the set of topics a current or potential peer
// is subscribed to based on key/value pairs from the ENR record.
type TopicExtractor func(ctx context.Context, node *enode.Node) ([]string, error)

// PeerFilterFunc defines the filtering interface used by the crawler to decide if a node
// is a valid candidate to index in the crawler.
type PeerFilterFunc func(*enode.Node) bool

type Crawler interface {
	Start(te TopicExtractor) error
	Stop()
	RemovePeerId(peerID peer.ID)
	RemoveTopic(topic Topic)
	PeersForTopic(topic Topic) []*enode.Node
}
