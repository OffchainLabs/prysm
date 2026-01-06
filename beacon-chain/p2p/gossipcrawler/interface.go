package gossipcrawler

import (
	"context"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TopicExtractor is a function that can determine the set of topics a current or potential peer
// is subscribed to based on key/value pairs from the ENR record.
type TopicExtractor func(ctx context.Context, node *enode.Node) ([]string, error)

// PeerFilterFunc defines the filtering interface used by the crawler to decide if a node
// is a valid candidate to index in the crawler.
type PeerFilterFunc func(*enode.Node) bool

type Crawler interface {
	Start(te TopicExtractor) error
	RemovePeerByPeerId(peerID peer.ID)
	RemoveTopic(topic string)
	PeersForTopic(topic string) []*enode.Node
	TriggerCrawl()
}

// SubnetTopicsProvider returns the set of gossipsub topics the node
// should currently maintain peer connections for along with the minimum number of peers required
// for each topic.
type SubnetTopicsProvider func() map[string]int

// GossipDialer controls dialing peers for gossipsub topics based
// on a provided SubnetTopicsProvider and the p2p crawler.
type GossipDialer interface {
	Start(provider SubnetTopicsProvider) error
	DialPeersForTopicBlocking(ctx context.Context, topic string, nPeers int) error
	ProtectedPeers() []peer.ID
}
