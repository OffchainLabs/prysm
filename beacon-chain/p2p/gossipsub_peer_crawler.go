package p2p

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/gossipsubcrawler"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/ethereum/go-ethereum/p2p/enode"
)

type peerNode struct {
	id       enode.ID
	isPinged bool
	node     *enode.Node
	peerID   peer.ID
	topics   map[gossipsubcrawler.Topic]struct{}
}

type crawledPeers struct {
	g *GossipsubPeerCrawler

	mu       sync.RWMutex
	byEnode  map[enode.ID]*peerNode
	byPeerId map[peer.ID]*peerNode
	byTopic  map[gossipsubcrawler.Topic]map[peer.ID]struct{}
}

func (cp *crawledPeers) updateStatusToPinged(enodeID enode.ID) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	existingPNode, ok := cp.byEnode[enodeID]
	if !ok {
		return
	}

	// we only want to ping a node with a given NodeId once -> not on every sequence number change
	// as ping is simply a test of a node being reachable and not fake
	existingPNode.isPinged = true
}

func (cp *crawledPeers) removePeerOnPingFailure(enodeID enode.ID) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	existingPNode, ok := cp.byEnode[enodeID]
	if !ok {
		return
	}

	// same idea as in "updateStatusToPinged" above.
	// We don't want to test pings for every sequence number change for a given node as that
	// can lead to an explosion in the number of pings the crawler needs to do.
	// So, remove the peer when the first ping fails. If the node becomes reachable later,
	// we will discover it during a re-crawl and ping it again to test for reachability.
	// we're not blacklisting this peer anyways.
	cp.updateTopicsUnlocked(existingPNode, nil)
}

func (cp *crawledPeers) updateCrawledIfNewer(node *enode.Node, topics []string) {
	cp.mu.Lock()

	enodeID := node.ID()
	existingPNode, ok := cp.byEnode[enodeID]

	if ok && existingPNode.node == nil {
		log.WithField("enodeId", enodeID).Error("enode is nil for enodeId")
		cp.mu.Unlock()
		return
	}

	// we don't want to update enodes with a lower sequence number as they're stale records
	if ok && existingPNode.node.Seq() >= node.Seq() {
		cp.mu.Unlock()
		return
	}

	if !ok {
		// this is a new peer
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

	if existingPNode.isPinged || len(topics) == 0 {
		cp.mu.Unlock()
		return
	}
	cp.mu.Unlock()

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
			// remove the peer if it has no more topics left
			if len(pnode.topics) == 0 {
				cp.updateTopicsUnlocked(pnode, nil)
			}
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

	peerFilter gossipsubcrawler.PeerFilterFunc
	scorer     PeerScoreFunc

	maxConcurrentPings int
	pingCh             chan enode.Node
	pingSemaphore      *semaphore.Weighted

	wg   sync.WaitGroup
	once sync.Once
}

// cleanupInterval controls how frequently we sweep crawled peers and prune
// those that are no longer useful.
const cleanupInterval = 5 * time.Minute

// PeerScoreFunc provides a way to calculate a score for a given peer ID.
// Higher scores should indicate better peers.
type PeerScoreFunc func(peer.ID) float64

func NewGossipsubPeerCrawler(
	service *Service,
	dv5 ListenerRebooter,
	crawlTimeout time.Duration,
	crawlInterval time.Duration,
	maxConcurrentPings int,
	peerFilter gossipsubcrawler.PeerFilterFunc,
	scorer PeerScoreFunc,
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
	if peerFilter == nil {
		return nil, errors.New("peer filter is nil")
	}
	if scorer == nil {
		return nil, errors.New("peer scorer is nil")
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
		peerFilter:         peerFilter,
		scorer:             scorer,
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

	var peerNodes []*peerNode
	seen := make(map[enode.ID]bool)
	for peerID := range peerIDs {
		peerNode, ok := g.crawledPeers.byPeerId[peerID]
		if !ok {
			continue
		}
		if peerNode.isPinged && g.peerFilter(peerNode.node) {
			// Skip if we've already seen this enode ID
			if seen[peerNode.id] {
				continue
			}
			seen[peerNode.id] = true
			peerNodes = append(peerNodes, peerNode)
		}
	}

	// Sort peerNodes in descending order of their scores.
	sort.Slice(peerNodes, func(i, j int) bool {
		scoreI := g.scorer(peerNodes[i].peerID)
		scoreJ := g.scorer(peerNodes[j].peerID)
		return scoreI > scoreJ
	})

	nodes := make([]*enode.Node, len(peerNodes))
	for i, pn := range peerNodes {
		nodes[i] = pn.node
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
		g.wg.Go(func() {
			g.cleanupLoop()
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
					g.crawledPeers.removePeerOnPingFailure(node.ID())
					return
				}

				g.crawledPeers.updateStatusToPinged(node.ID())
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

		if !g.peerFilter(node) {
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

// cleanupLoop periodically removes peers that the filter rejects or that
// have no topics of interest. It uses the same context lifecycle as other
// background loops.
func (g *GossipsubPeerCrawler) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	// Initial cleanup to catch any leftovers from startup state
	g.cleanup()

	for {
		select {
		case <-ticker.C:
			g.cleanup()
		case <-g.ctx.Done():
			return
		}
	}
}

// cleanup scans the crawled peer set and removes entries that either fail
// the current peer filter or have no topics of interest remaining.
func (g *GossipsubPeerCrawler) cleanup() {
	cp := g.crawledPeers

	// Snapshot current peers to evaluate without holding the lock during
	// filter and topic extraction.
	cp.mu.RLock()
	peers := make([]*peerNode, 0, len(cp.byPeerId))
	for _, p := range cp.byPeerId {
		peers = append(peers, p)
	}
	cp.mu.RUnlock()

	for _, p := range peers {
		// Remove peers that no longer pass the filter
		if !g.peerFilter(p.node) {
			cp.removePeer(p.id)
			continue
		}

		// Re-extract topics; if the extractor errors or yields none, drop the peer.
		topics, err := g.topicExtractor(g.ctx, p.node)
		if err != nil || len(topics) == 0 {
			cp.removePeer(p.id)
		}
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
