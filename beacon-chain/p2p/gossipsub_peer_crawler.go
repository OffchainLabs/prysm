package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/gossipsubcrawler"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/ethereum/go-ethereum/p2p/enode"
)

type peerStatus int

const (
	peerStatusUnknown peerStatus = iota
	peerStatusCrawled
	peerStatusPinged
)

type peerNode struct {
	id     enode.ID
	status peerStatus
	node   *enode.Node
	peerID peer.ID
	topics map[gossipsubcrawler.Topic]struct{}
}

type crawledPeers struct {
	g  *GossipsubPeerCrawler
	mu sync.RWMutex

	byEnode  map[enode.ID]*peerNode
	byPeerId map[peer.ID]*peerNode
	byTopic  map[gossipsubcrawler.Topic]map[peer.ID]struct{}
}

func (cp *crawledPeers) updateStatusToPinged(node *enode.Node) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	enodeID := node.ID()
	existingPNode, ok := cp.byEnode[enodeID]
	if !ok {
		return
	}
	if existingPNode.node.Seq() == node.Seq() {
		existingPNode.status = peerStatusPinged
		return
	}
}

func (cp *crawledPeers) removePeerOnPingFailure(node *enode.Node) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	enodeID := node.ID()
	existingPNode, ok := cp.byEnode[enodeID]
	if !ok {
		return
	}
	if existingPNode.node.Seq() == node.Seq() {
		cp.updateTopicsUnlocked(existingPNode, nil)
	}
}

func (cp *crawledPeers) updateCrawledIfNewer(node *enode.Node, topics []string) {
	cp.mu.Lock()

	enodeID := node.ID()
	existingPNode, ok := cp.byEnode[enodeID]
	if ok && existingPNode.node.Seq() >= node.Seq() {
		cp.mu.Unlock()
		return
	}
	if !ok {
		peerID, err := enodeToPeerID(node)
		if err != nil {
			log.WithError(err).WithField("node", node.ID()).Debug("Failed to convert enode to peer ID")
			cp.mu.Unlock()
			return
		}
		existingPNode = &peerNode{
			id:     enodeID,
			node:   node,
			peerID: peerID,
			topics: make(map[gossipsubcrawler.Topic]struct{}),
		}
		cp.byEnode[enodeID] = existingPNode
		cp.byPeerId[peerID] = existingPNode
	} else {
		existingPNode.node = node
	}

	cp.updateTopicsUnlocked(existingPNode, topics)
	cp.mu.Unlock()
	if len(topics) == 0 {
		return
	}
	select {
	case cp.g.pingCh <- *node:
	case <-cp.g.ctx.Done():
		return
	}
}

func (cp *crawledPeers) removeTopic(topic gossipsubcrawler.Topic) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Get all peers subscribed to this topic
	peers, ok := cp.byTopic[topic]
	if !ok {
		return // Topic doesn't exist
	}

	// Remove the topic from each peer's topic list
	for peerID := range peers {
		if pnode, exists := cp.byPeerId[peerID]; exists {
			delete(pnode.topics, topic)
		}
	}

	// Remove the topic from byTopic map
	delete(cp.byTopic, topic)
}

func (cp *crawledPeers) removePeerId(peerID peer.ID) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	pnode, exists := cp.byPeerId[peerID]
	if !exists {
		return
	}

	// Use updateTopicsUnlocked with empty topics to remove the peer
	cp.updateTopicsUnlocked(pnode, nil)
}

func (cp *crawledPeers) removePeer(enodeID enode.ID) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	pnode, exists := cp.byEnode[enodeID]
	if !exists {
		return
	}
	cp.updateTopicsUnlocked(pnode, nil)
}

// setting topics to empty will remove the peer completely.
func (cp *crawledPeers) updateTopicsUnlocked(pnode *peerNode, topics []string) {
	// If topics is empty, remove the peer completely.
	if len(topics) == 0 {
		delete(cp.byPeerId, pnode.peerID)
		delete(cp.byEnode, pnode.id)
		for t := range pnode.topics {
			if peers, ok := cp.byTopic[t]; ok {
				delete(peers, pnode.peerID)
				if len(peers) == 0 {
					delete(cp.byTopic, t)
				}
			}
		}
		pnode.topics = nil // Clear topics to indicate removal.
		return
	}

	newTopics := make(map[gossipsubcrawler.Topic]struct{})
	for _, t := range topics {
		newTopics[gossipsubcrawler.Topic(t)] = struct{}{}
	}

	// Remove old topics that are no longer present.
	for oldTopic := range pnode.topics {
		if _, exists := newTopics[oldTopic]; !exists {
			if peers, ok := cp.byTopic[oldTopic]; ok {
				delete(peers, pnode.peerID)
				if len(peers) == 0 {
					delete(cp.byTopic, oldTopic)
				}
			}
		}
	}

	// Add new topics.
	for newTopic := range newTopics {
		if _, exists := pnode.topics[newTopic]; !exists {
			if _, ok := cp.byTopic[newTopic]; !ok {
				cp.byTopic[newTopic] = make(map[peer.ID]struct{})
			}
			cp.byTopic[newTopic][pnode.peerID] = struct{}{}
		}
	}
	pnode.topics = newTopics
}

type GossipsubPeerCrawler struct {
	ctx    context.Context
	cancel context.CancelFunc

	crawlInterval time.Duration
	crawlTimeout  time.Duration

	crawledPeers *crawledPeers

	// Discovery interface for finding peers
	dv5 ListenerRebooter

	service *Service

	topicExtractor gossipsubcrawler.TopicExtractor

	maxConcurrentPings int
	pingCh             chan enode.Node
	pingSemaphore      *semaphore.Weighted

	wg   sync.WaitGroup
	once sync.Once
}

func NewGossipsubPeerCrawler(
	service *Service,
	dv5 ListenerRebooter,
	crawlTimeout time.Duration,
	crawlInterval time.Duration,
	maxConcurrentPings int,
) (*GossipsubPeerCrawler, error) {
	if service == nil {
		return nil, errors.New("service is nil")
	}
	if dv5 == nil {
		return nil, errors.New("dv5 is nil")
	}
	if crawlTimeout <= 0 {
		return nil, errors.New("crawl timeout must be greater than 0")
	}
	if crawlInterval <= 0 {
		return nil, errors.New("crawl interval must be greater than 0")
	}
	if maxConcurrentPings <= 0 {
		return nil, errors.New("max concurrent pings must be greater than 0")
	}

	ctx, cancel := context.WithCancel(context.Background())
	g := &GossipsubPeerCrawler{
		ctx:                ctx,
		cancel:             cancel,
		crawlInterval:      crawlInterval,
		crawlTimeout:       crawlTimeout,
		service:            service,
		dv5:                dv5,
		maxConcurrentPings: maxConcurrentPings,
	}
	g.pingCh = make(chan enode.Node, 4*g.maxConcurrentPings)
	g.pingSemaphore = semaphore.NewWeighted(int64(g.maxConcurrentPings))
	g.crawledPeers = &crawledPeers{
		g:        g,
		byEnode:  make(map[enode.ID]*peerNode),
		byPeerId: make(map[peer.ID]*peerNode),
		byTopic:  make(map[gossipsubcrawler.Topic]map[peer.ID]struct{}),
	}
	return g, nil
}
func (g *GossipsubPeerCrawler) PeersForTopic(topic gossipsubcrawler.Topic) []*enode.Node {
	g.crawledPeers.mu.RLock()
	defer g.crawledPeers.mu.RUnlock()

	peerIDs, ok := g.crawledPeers.byTopic[topic]
	if !ok {
		return nil
	}

	var nodes []*enode.Node
	for peerID := range peerIDs {
		peerNode, ok := g.crawledPeers.byPeerId[peerID]
		if !ok {
			continue
		}
		if peerNode.status == peerStatusPinged {
			nodes = append(nodes, peerNode.node)
		}
	}

	return nodes
}

func (g *GossipsubPeerCrawler) RemovePeerId(peerID peer.ID) {
	g.crawledPeers.removePeerId(peerID)
}

func (g *GossipsubPeerCrawler) RemoveTopic(topic gossipsubcrawler.Topic) {
	g.crawledPeers.removeTopic(topic)
}

// Start runs the crawler's loops in the background.
func (g *GossipsubPeerCrawler) Start(te gossipsubcrawler.TopicExtractor) error {
	if te == nil {
		return errors.New("topic extractor is nil")
	}
	g.once.Do(func() {
		g.topicExtractor = te
		g.wg.Go(func() {
			g.crawlLoop()
		})
		g.wg.Go(func() {
			g.pingLoop()
		})
	})

	return nil
}

// Stop terminates the crawler.
func (g *GossipsubPeerCrawler) Stop() {
	g.cancel()
	g.wg.Wait()
}

func (g *GossipsubPeerCrawler) pingLoop() {
	for {
		select {
		case node := <-g.pingCh:
			if err := g.pingSemaphore.Acquire(g.ctx, 1); err != nil {
				return // Context cancelled, exit loop.
			}
			go func(node *enode.Node) {
				defer g.pingSemaphore.Release(1)

				if err := g.dv5.Ping(node); err != nil {
					g.crawledPeers.removePeerOnPingFailure(node)
					return
				}

				g.crawledPeers.updateStatusToPinged(node)
			}(&node)

		case <-g.ctx.Done():
			return
		}
	}
}

func (g *GossipsubPeerCrawler) crawlLoop() {
	ticker := time.NewTicker(g.crawlInterval)
	defer ticker.Stop()

	g.crawl()
	for {
		select {
		case <-ticker.C:
			g.crawl()
		case <-g.ctx.Done():
			return
		}
	}
}

func (g *GossipsubPeerCrawler) crawl() {
	ctx, cancel := context.WithTimeout(g.ctx, g.crawlTimeout)
	defer cancel()

	iterator := g.dv5.RandomNodes()

	// Ensure iterator unblocks on context cancellation or timeout
	go func() {
		<-ctx.Done()
		iterator.Close()
	}()

	for iterator.Next() {
		if ctx.Err() != nil {
			return
		}

		node := iterator.Node()
		if node == nil {
			continue
		}

		if !g.service.filterPeer(node) {
			g.crawledPeers.removePeer(node.ID())
			continue
		}

		topics, err := g.topicExtractor(ctx, node)
		if err != nil {
			log.WithError(err).WithField("node", node.ID()).Debug("Failed to extract topics, skipping")
			continue
		}

		g.crawledPeers.updateCrawledIfNewer(node, topics)
	}
}

// enodeToPeerID converts an enode record to a peer ID.
func enodeToPeerID(n *enode.Node) (peer.ID, error) {
	info, _, err := convertToAddrInfo(n)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "", errors.New("peer info is nil")
	}
	return info.ID, nil
}
