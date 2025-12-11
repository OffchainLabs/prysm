package p2p

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/gossipsubcrawler"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	peerPerTopic = 20
	dialInterval = 1 * time.Second
)

type GossipsubPeerDialer struct {
	ctx    context.Context
	cancel context.CancelFunc

	listPeers func(topic string) []peer.ID
	dialPeers func(ctx context.Context, maxConcurrentDials int, nodes []*enode.Node) uint

	crawler        gossipsubcrawler.Crawler
	topicsProvider gossipsubcrawler.SubnetTopicsProvider

	wg   sync.WaitGroup
	once sync.Once
}

func NewGossipsubPeerDialer(
	crawler gossipsubcrawler.Crawler,
	listPeers func(topic string) []peer.ID,
	dialPeers func(ctx context.Context, maxConcurrentDials int, nodes []*enode.Node) uint,
) *GossipsubPeerDialer {
	ctx, cancel := context.WithCancel(context.Background())
	return &GossipsubPeerDialer{
		listPeers: listPeers,
		dialPeers: dialPeers,
		crawler:   crawler,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (g *GossipsubPeerDialer) Stop() {
	g.cancel()
	g.wg.Wait()
}

func (g *GossipsubPeerDialer) Start(provider gossipsubcrawler.SubnetTopicsProvider) error {
	g.once.Do(func() {
		g.topicsProvider = provider
		g.wg.Go(func() {
			g.dialLoop()
		})
	})

	return nil
}

func (g *GossipsubPeerDialer) dialLoop() {
	ticker := time.NewTicker(dialInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			topics := g.topicsProvider()
			var peersToDial []*enode.Node

			for _, topic := range topics {
				newPeers := g.peersForTopic(topic, peerPerTopic)
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

func (g *GossipsubPeerDialer) DialPeersForTopicBlocking(ctx context.Context, topic string, nPeers int) error {
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

func (g *GossipsubPeerDialer) peersForTopic(topic string, targetCount int) []*enode.Node {
	peers := g.listPeers(topic)
	peerCount := len(peers)
	if peerCount >= targetCount {
		return nil
	}
	missing := targetCount - peerCount
	// this is fine as "PeersForTopic" does not return peers we are already connected to
	newPeers := g.crawler.PeersForTopic(topic)
	if len(newPeers) > missing {
		newPeers = newPeers[:missing]
	}

	return newPeers
}

func (g *GossipsubPeerDialer) dialPeersWithRatelimiting(peers []*enode.Node) {
	// Dial new peers in batches.
	maxConcurrentDials := math.MaxInt
	if flags.MaxDialIsActive() {
		maxConcurrentDials = flags.Get().MaxConcurrentDials
	}
	g.dialPeers(g.ctx, maxConcurrentDials, peers)
}
