package p2p

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/gossipcrawler"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
)

const dialInterval = 1 * time.Second

// GossipPeerDialer maintains minimum peer counts for gossip topics by periodically
// dialing new peers discovered by a crawler. It runs a background loop that checks each
// topic's peer count and dials new peers when below the target threshold.
type GossipPeerDialer struct {
	ctx context.Context

	listPeers func(topic string) []peer.ID
	dialPeers func(ctx context.Context, maxConcurrentDials int, nodes []*enode.Node) uint

	crawler        gossipcrawler.Crawler
	topicsProvider gossipcrawler.SubnetTopicsProvider

	once sync.Once
}

// NewGossipPeerDialer creates a new GossipPeerDialer instance.
//
// Parameters:
//   - ctx: Parent context that controls the lifecycle of the dialer. When cancelled,
//     the background dial loop will terminate.
//   - crawler: Source of peer candidates for each topic. The crawler maintains a registry
//     of peers discovered through DHT crawling, indexed by the topics they subscribe to.
//   - listPeers: Function that returns the current peers connected for a given topic.
//     Used to determine how many additional peers need to be dialed.
//   - dialPeers: Function that dials the given enode.Node peers with a concurrency limit.
//     Returns the number of successful dials.
//
// The dialer must be started with Start() before it begins maintaining peer counts.
func NewGossipPeerDialer(
	ctx context.Context,
	crawler gossipcrawler.Crawler,
	listPeers func(topic string) []peer.ID,
	dialPeers func(ctx context.Context, maxConcurrentDials int, nodes []*enode.Node) uint,
) *GossipPeerDialer {
	return &GossipPeerDialer{
		ctx:       ctx,
		listPeers: listPeers,
		dialPeers: dialPeers,
		crawler:   crawler,
	}
}

// Start begins the background dial loop that maintains peer counts for all topics.
//
// The provider function is called on each tick to get the current list of topics that
// need peer maintenance. This allows the set of topics to change dynamically as the node
// subscribes/unsubscribes from subnets.
//
// Start is idempotent - calling it multiple times has no effect after the first call.
// Only the provider from the first call will be used; subsequent calls are ignored.
//
// The dial loop runs every dialInterval (1 second) and for each topic:
//  1. Checks current peer count via listPeers()
//  2. If below the per-topic min peer count, requests candidates from the crawler
//  3. Deduplicates peers across all topics to avoid redundant dials
//  4. Dials missing peers with rate limiting if enabled
//
// Returns nil always (error return preserved for interface compatibility).
func (g *GossipPeerDialer) Start(provider gossipcrawler.SubnetTopicsProvider) error {
	g.once.Do(func() {
		g.topicsProvider = provider
		go g.dialLoop()
	})

	return nil
}

func (g *GossipPeerDialer) dialLoop() {
	ticker := time.NewTicker(dialInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var peersToDial []*enode.Node

			topicsWithMinPeers := g.topicsProvider()
			for topic, minPeers := range topicsWithMinPeers {
				newPeers := g.peersForTopic(topic, minPeers)
				peersToDial = append(peersToDial, newPeers...)
			}

			if len(peersToDial) == 0 {
				continue
			}

			// Deduplicate peers to avoid dialing the same peer multiple times.
			uniquePeers := make([]*enode.Node, 0, len(peersToDial))
			seen := make(map[enode.ID]struct{})
			for _, p := range peersToDial {
				if _, ok := seen[p.ID()]; !ok {
					seen[p.ID()] = struct{}{}
					uniquePeers = append(uniquePeers, p)
				}
			}
			peersToDial = uniquePeers

			g.dialPeersWithRatelimiting(peersToDial)

		case <-g.ctx.Done():
			return
		}
	}
}

// DialPeersForTopicBlocking blocks until the specified topic has at least nPeers connected,
// or until the context is cancelled.
//
// This method is useful when you need to ensure a minimum number of peers are connected
// for a specific topic before proceeding (e.g., before publishing a message).
//
// The method polls in a loop:
//  1. Check if current peer count >= nPeers, return nil if satisfied
//  2. Get peer candidates from crawler for this topic
//  3. Dial candidates with rate limiting
//  4. Wait 100ms for connections to establish in pubsub layer
//  5. Repeat until target reached or context cancelled
//
// Parameters:
//   - ctx: Context to cancel the blocking operation. Takes precedence for cancellation.
//   - topic: The gossipsub topic to ensure peers for.
//   - nPeers: Minimum number of peers required before returning.
//
// Returns:
//   - nil: Successfully reached the target peer count.
//   - ctx.Err(): The provided context was cancelled.
//   - g.ctx.Err(): The dialer's parent context was cancelled.
//
// Note: This may block indefinitely if the crawler cannot provide enough peers
// and the context has no deadline.
func (g *GossipPeerDialer) DialPeersForTopicBlocking(ctx context.Context, topic string, nPeers int) error {
	for {
		peers := g.listPeers(topic)
		if len(peers) >= nPeers {
			return nil
		}

		newPeers := g.peersForTopic(topic, nPeers)
		if len(newPeers) > 0 {
			g.dialPeersWithRatelimiting(newPeers)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
			// some wait here is good after dialing as connections take some time to show up in pubsub
		case <-time.After(100 * time.Millisecond):
		case <-g.ctx.Done():
			return g.ctx.Err()
		}
	}
}

func (g *GossipPeerDialer) peersForTopic(topic string, targetCount int) []*enode.Node {
	peers := g.listPeers(topic)
	peerCount := len(peers)
	if peerCount >= targetCount {
		return nil
	}
	missing := targetCount - peerCount
	newPeers := g.crawler.PeersForTopic(topic)
	if len(newPeers) > missing {
		newPeers = newPeers[:missing]
	}

	return newPeers
}

func (g *GossipPeerDialer) dialPeersWithRatelimiting(peers []*enode.Node) {
	// Dial new peers in batches.
	maxConcurrentDials := math.MaxInt
	if flags.MaxDialIsActive() {
		maxConcurrentDials = flags.Get().MaxConcurrentDials
	}
	g.dialPeers(g.ctx, maxConcurrentDials, peers)
}
