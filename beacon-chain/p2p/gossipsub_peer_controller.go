package p2p

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/gossipsubcrawler"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/pkg/errors"
)

var (
	peerPerTopic = 20
)

type GossipsubPeerDialer struct {
	ctx    context.Context
	cancel context.CancelFunc

	service *Service

	crawler        gossipsubcrawler.Crawler
	topicsProvider gossipsubcrawler.SubnetTopicsProvider

	wg   sync.WaitGroup
	once sync.Once
}

func NewGossipsubPeerDialer(service *Service, crawler gossipsubcrawler.Crawler) *GossipsubPeerDialer {
	ctx, cancel := context.WithCancel(context.Background())
	return &GossipsubPeerDialer{
		service: service,
		crawler: crawler,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (g *GossipsubPeerDialer) Stop() {
	g.cancel()
	g.wg.Wait()
}

func (g *GossipsubPeerDialer) Start(provider gossipsubcrawler.SubnetTopicsProvider) error {
	if provider == nil {
		return errors.New("topics provider is nil")
	}

	g.once.Do(func() {
		g.topicsProvider = provider
		g.wg.Go(func() {
			g.dialLoop()
		})
	})

	return nil
}

func (g *GossipsubPeerDialer) dialLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			topics := g.topicsProvider()
			var peersToDial []*enode.Node

			for _, topic := range topics {
				peers := g.service.PubSub().ListPeers(topic)
				peerCount := len(peers)
				if peerCount >= peerPerTopic {
					continue
				}
				missing := peerPerTopic - peerCount
				// this is fine as "PeersForTopic" does not return peers we are already connected to
				newPeers := g.crawler.PeersForTopic(gossipsubcrawler.Topic(topic))
				if len(newPeers) > missing {
					newPeers = newPeers[:missing]
				}
				peersToDial = append(peersToDial, newPeers...)
			}

			if len(peersToDial) > 0 {
				// Dial new peers in batches.
				maxConcurrentDials := math.MaxInt
				if flags.MaxDialIsActive() {
					maxConcurrentDials = flags.Get().MaxConcurrentDials
				}
				g.service.DialPeers(g.ctx, maxConcurrentDials, peersToDial)
			}

		case <-g.ctx.Done():
			return
		}
	}
}

func (g *GossipsubPeerDialer) DialPeersForTopicBlocking(topic string, nPeers int) error {
	for {
		peers := g.service.PubSub().ListPeers(topic)
		if len(peers) >= nPeers {
			return nil
		}

		missing := nPeers - len(peers)
		// this is fine as "PeersForTopic" does not return peers we are already connected to
		newPeers := g.crawler.PeersForTopic(gossipsubcrawler.Topic(topic))
		if len(newPeers) > 0 {
			if len(newPeers) > missing {
				newPeers = newPeers[:missing]
			}
			maxConcurrentDials := math.MaxInt
			if flags.MaxDialIsActive() {
				maxConcurrentDials = flags.Get().MaxConcurrentDials
			}
			g.service.DialPeers(g.ctx, maxConcurrentDials, newPeers)
		}

		select {
		case <-time.After(100 * time.Millisecond):
		case <-g.ctx.Done():
			return g.ctx.Err()
		}
	}
}
