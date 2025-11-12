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

type Crawler interface {
	Start(te TopicExtractor) error
	Stop()
	RemovePeerId(peerID peer.ID)
	RemoveTopic(topic Topic)
	PeersForTopic(topic Topic) []*enode.Node
}
