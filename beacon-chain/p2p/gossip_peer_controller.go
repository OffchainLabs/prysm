package p2p

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/gossipcrawler"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
)

const dialInterval = 1 * time.Second
const peerCountLogInterval = 1 * time.Minute
const topicMonitorInterval = 1 * time.Second

// GossipPeerDialer maintains minimum peer counts for gossip topics by periodically
// dialing new peers discovered by a crawler. It runs a background loop that checks each
// topic's peer count and dials new peers when below the target threshold.
type GossipPeerDialer struct {
	ctx context.Context

	listPeers func(topic string) []peer.ID
	dialPeers func(ctx context.Context, maxConcurrentDials int, nodes []*enode.Node) uint

	crawler        gossipcrawler.Crawler
	topicsProvider gossipcrawler.SubnetTopicsProvider

	cachedTopics map[string]int

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
		g.cachedTopics = make(map[string]int)
		go g.dialLoop()
		go g.logPeerCountsLoop()
		go g.topicMonitorLoop()
	})

	return nil
}

func (g *GossipPeerDialer) dialLoop() {
	ticker := time.NewTicker(dialInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			peersToDial := g.selectPeersForTopics()
			if len(peersToDial) == 0 {
				continue
			}
			g.dialPeersWithRatelimiting(peersToDial)

		case <-g.ctx.Done():
			return
		}
	}
}

func (g *GossipPeerDialer) logPeerCountsLoop() {
	ticker := time.NewTicker(peerCountLogInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			topics := g.topicsProvider()
			for topic, minPeers := range topics {
				currentPeers := len(g.listPeers(topic))
				log.WithField("topic", topic).
					WithField("currentPeers", currentPeers).
					WithField("minPeers", minPeers).
					Info("Gossip topic peer count")
			}

		case <-g.ctx.Done():
			return
		}
	}
}

// topicsChanged compares the new topics with cached topics and returns true
// if topics have been added or removed. Changes to min peer counts are ignored.
func (g *GossipPeerDialer) topicsChanged(newTopics map[string]int) bool {
	if len(newTopics) != len(g.cachedTopics) {
		return true
	}
	for topic := range newTopics {
		if _, ok := g.cachedTopics[topic]; !ok {
			return true
		}
	}
	return false
}

func (g *GossipPeerDialer) topicMonitorLoop() {
	ticker := time.NewTicker(topicMonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			topics := g.topicsProvider()
			if g.topicsChanged(topics) {
				g.cachedTopics = topics
				g.crawler.TriggerCrawl()
			}

		case <-g.ctx.Done():
			return
		}
	}
}

// selectPeersForTopics builds a bidirectional mapping of topics to peers and selects
// peers to dial using a greedy algorithm that prioritizes peers serving multiple topics.
// When a peer is selected, the needed count is decremented for ALL topics that peer serves,
// avoiding redundant dials when one peer can satisfy multiple topic requirements.
func (g *GossipPeerDialer) selectPeersForTopics() []*enode.Node {
	topicsWithMinPeers := g.topicsProvider()

	// Calculate how many peers each topic still needs.
	neededByTopic := make(map[string]int)
	for topic, minPeers := range topicsWithMinPeers {
		currentCount := len(g.listPeers(topic))
		if needed := minPeers - currentCount; needed > 0 {
			neededByTopic[topic] = needed
		}
	}

	if len(neededByTopic) == 0 {
		return nil
	}

	peerToTopics := make(map[enode.ID][]string)
	nodeByID := make(map[enode.ID]*enode.Node)

	for topic := range neededByTopic {
		candidates := g.crawler.PeersForTopic(topic)
		for _, node := range candidates {
			id := node.ID()
			if _, exists := nodeByID[id]; !exists {
				nodeByID[id] = node
			}
			peerToTopics[id] = append(peerToTopics[id], topic)
		}
	}

	// Build candidate list sorted by topic count (descending).
	// Peers serving more topics are prioritized.
	type candidate struct {
		node   *enode.Node
		topics []string
	}
	candidates := make([]candidate, 0, len(peerToTopics))
	for id, topics := range peerToTopics {
		candidates = append(candidates, candidate{node: nodeByID[id], topics: topics})
	}

	// sort candidates by topic count (descending)
	slices.SortFunc(candidates, func(a, b candidate) int {
		return len(b.topics) - len(a.topics)
	})

	// Greedy selection with cross-topic accounting.
	var selected []*enode.Node
	for _, c := range candidates {
		// Check if this peer serves any topic we still need.
		servesNeededTopic := false
		for _, topic := range c.topics {
			if neededByTopic[topic] > 0 {
				servesNeededTopic = true
				break
			}
		}

		if !servesNeededTopic {
			continue
		}

		// Select this peer and decrement needed count for ALL topics it serves.
		selected = append(selected, c.node)
		for _, topic := range c.topics {
			if neededByTopic[topic] > 0 {
				neededByTopic[topic]--
			}
		}
	}

	return selected
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

// SoleProviderPeers returns peer IDs that are the sole provider for at least one topic.
// A peer is considered a sole provider if:
// 1. It's the only connected peer for a topic (listPeers returns only this peer)
// 2. The crawler has no other known peers for that topic
//
// These peers should be protected from pruning since losing them would mean
// losing connectivity to that topic entirely.
func (g *GossipPeerDialer) SoleProviderPeers() []peer.ID {
	if g.topicsProvider == nil {
		return nil
	}

	topics := g.topicsProvider()
	soleProviders := make(map[peer.ID]struct{})

	for topic := range topics {
		connectedPeers := g.listPeers(topic)

		// Skip if no peers or more than one connected peer
		if len(connectedPeers) != 1 {
			continue
		}

		// Check if crawler knows of any other peers for this topic
		crawlerPeers := g.crawler.PeersForTopic(topic)
		if len(crawlerPeers) == 0 {
			// This peer is the sole known provider
			soleProviders[connectedPeers[0]] = struct{}{}
		}
	}

	result := make([]peer.ID, 0, len(soleProviders))
	for pid := range soleProviders {
		result = append(result, pid)
	}
	return result
}

func (g *GossipPeerDialer) dialPeersWithRatelimiting(peers []*enode.Node) {
	// Dial new peers in batches.
	maxConcurrentDials := math.MaxInt
	if flags.MaxDialIsActive() {
		maxConcurrentDials = flags.Get().MaxConcurrentDials
	}
	g.dialPeers(g.ctx, maxConcurrentDials, peers)
}
