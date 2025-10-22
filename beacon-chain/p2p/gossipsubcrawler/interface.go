package gossipsubcrawler

import (
	"context"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Crawler is a dependency-light interface
// for the gossip peer crawler used by sync. Keeping this in an inner
// package avoids circular imports between p2p and its testing helpers.
type Crawler interface {
	Start(topicExtractor func(ctx context.Context, node *enode.Node) ([]string, error)) error
	Stop()
	RemoveTopic(topic string)
	RemovePeer(enodeID enode.ID)
	RemovePeerByPeerID(pid peer.ID)
	PeersForTopic(topic string) []*enode.Node
}
