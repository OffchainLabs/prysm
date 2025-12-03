package p2p

import (
	"context"
	"fmt"
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
	isPinged bool
	node     *enode.Node
	peerID   peer.ID
	topics   map[string]struct{}
}

type crawledPeers struct {
	mu              sync.RWMutex
	peerNodeByEnode map[enode.ID]*peerNode
	peerNodeByPid   map[peer.ID]*peerNode
	pidsByTopic     map[string]map[peer.ID]struct{}
}

func (cp *crawledPeers) updateStatusToPinged(enodeID enode.ID) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	existingPNode, ok := cp.peerNodeByEnode[enodeID]
	if !ok {
		return
	}

	// we only want to ping a node with a given NodeId once -> not on every sequence number change
	// as ping is simply a test of a node being reachable and not fake
	existingPNode.isPinged = true
}

func (cp *crawledPeers) updateCrawledIfNewer(node *enode.Node, topics []string) (shouldPing bool, err error) {
	if node == nil {
		return false, errors.New("node is nil")
	}

	return cp.updatePeer(node, topics)
}

func (cp *crawledPeers) updatePeer(node *enode.Node, topics []string) (shouldPing bool, err error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	enodeID := node.ID()
	existingPNode, ok := cp.peerNodeByEnode[enodeID]

	if ok && existingPNode.node == nil {
		log.WithField("enodeId", enodeID).Error("enode is nil for enodeId")
		return false, errors.New("enode is nil for enodeId")
	}

	// we don't want to update enodes with a lower sequence number as they're stale records
	if ok && existingPNode.node.Seq() >= node.Seq() {
		return false, nil
	}

	if !ok {
		// this is a new peer
		peerID, err := enodeToPeerID(node)
		if err != nil {
			log.WithError(err).WithField("node", node.ID()).Debug("Failed to convert enode to peer ID")
			return false, errors.Wrap(err, "failed to convert enode to peer ID")
		}
		existingPNode = &peerNode{
			node:   node,
			peerID: peerID,
			topics: make(map[string]struct{}),
		}
		cp.peerNodeByEnode[enodeID] = existingPNode
		cp.peerNodeByPid[peerID] = existingPNode
	} else {
		existingPNode.node = node
	}

	cp.updateTopicsUnlocked(existingPNode, topics)

	if existingPNode.isPinged || len(topics) == 0 {
		return false, nil
	}
	return true, nil
}

func (cp *crawledPeers) removeTopic(topic string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Get all peers subscribed to this topic
	peers, ok := cp.pidsByTopic[topic]
	if !ok {
		return // Topic doesn't exist
	}

	// Remove the topic from each peer's topic list
	for peerID := range peers {
		if pnode, ok := cp.peerNodeByPid[peerID]; ok {
			delete(pnode.topics, topic)
			// remove the peer if it has no more topics left
			if len(pnode.topics) == 0 {
				cp.updateTopicsUnlocked(pnode, nil)
			}
		}
	}

	// Remove the topic from byTopic map
	delete(cp.pidsByTopic, topic)
}

func (cp *crawledPeers) removePeerByPeerId(peerID peer.ID) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	pnode, ok := cp.peerNodeByPid[peerID]
	if !ok {
		return
	}

	// Use updateTopicsUnlocked with empty topics to remove the peer
	cp.updateTopicsUnlocked(pnode, nil)
}

func (cp *crawledPeers) removePeerByNodeId(enodeID enode.ID) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	pnode, ok := cp.peerNodeByEnode[enodeID]
	if !ok {
		return
	}
	cp.updateTopicsUnlocked(pnode, nil)
}

func (cp *crawledPeers) cleanupPeer(pnode *peerNode) {
	delete(cp.peerNodeByPid, pnode.peerID)
	delete(cp.peerNodeByEnode, pnode.node.ID())
	for t := range pnode.topics {
		if peers, ok := cp.pidsByTopic[t]; ok {
			delete(peers, pnode.peerID)
			if len(peers) == 0 {
				delete(cp.pidsByTopic, t)
			}
		}
	}
	pnode.topics = nil // Clear topics to indicate removal.
}

func (cp *crawledPeers) removeOldTopicsFromPeer(pnode *peerNode, newTopics map[string]struct{}) {
	for oldTopic := range pnode.topics {
		if _, ok := newTopics[oldTopic]; !ok {
			if peers, ok := cp.pidsByTopic[oldTopic]; ok {
				delete(peers, pnode.peerID)
				if len(peers) == 0 {
					delete(cp.pidsByTopic, oldTopic)
				}
			}
		}
	}
}

func (cp *crawledPeers) addNewTopicsToPeer(pnode *peerNode, newTopics map[string]struct{}) {
	for newTopic := range newTopics {
		if _, ok := pnode.topics[newTopic]; !ok {
			if _, ok := cp.pidsByTopic[newTopic]; !ok {
				cp.pidsByTopic[newTopic] = make(map[peer.ID]struct{})
			}
			cp.pidsByTopic[newTopic][pnode.peerID] = struct{}{}
		}
	}
}

// updateTopicsUnlocked updates the topics associated with a peer node.
// If the topics slice is empty, the peer is completely removed from the crawled peers.
// Otherwise, it updates the peer's topics by removing old topics that are no longer
// present and adding new topics. This method assumes the caller holds the lock on cp.mu.
// If a topic has no peers after this update, it is removed from the list of topics we track peers for.
func (cp *crawledPeers) updateTopicsUnlocked(pnode *peerNode, topics []string) {
	// If topics is empty, remove the peer completely.
	if len(topics) == 0 {
		cp.cleanupPeer(pnode)
		return
	}

	newTopics := make(map[string]struct{})
	for _, t := range topics {
		newTopics[t] = struct{}{}
	}

	// Remove old topics that are no longer present.
	cp.removeOldTopicsFromPeer(pnode, newTopics)

	// Add new topics.
	cp.addNewTopicsToPeer(pnode, newTopics)

	pnode.topics = newTopics
}

type GossipsubPeerCrawler struct {
	ctx    context.Context
	cancel context.CancelFunc

	crawlInterval, crawlTimeout time.Duration

	crawledPeers *crawledPeers

	// Discovery interface for finding peers
	dv5 ListenerRebooter

	p2pSvc *Service

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
	p2pSvc *Service,
	dv5 ListenerRebooter,
	crawlTimeout time.Duration,
	crawlInterval time.Duration,
	maxConcurrentPings int,
	peerFilter gossipsubcrawler.PeerFilterFunc,
	scorer PeerScoreFunc,
) (*GossipsubPeerCrawler, error) {
	if p2pSvc == nil {
		return nil, errors.New("p2pSvc is nil")
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
		p2pSvc:             p2pSvc,
		dv5:                dv5,
		maxConcurrentPings: maxConcurrentPings,
		peerFilter:         peerFilter,
		scorer:             scorer,
	}
	g.pingCh = make(chan enode.Node, 4*g.maxConcurrentPings)
	g.pingSemaphore = semaphore.NewWeighted(int64(g.maxConcurrentPings))
	g.crawledPeers = &crawledPeers{
		peerNodeByEnode: make(map[enode.ID]*peerNode),
		peerNodeByPid:   make(map[peer.ID]*peerNode),
		pidsByTopic:     make(map[string]map[peer.ID]struct{}),
	}
	return g, nil
}

func (g *GossipsubPeerCrawler) PeersForTopic(topic string) []*enode.Node {
	g.crawledPeers.mu.RLock()
	defer g.crawledPeers.mu.RUnlock()

	peerIDs, ok := g.crawledPeers.pidsByTopic[topic]
	if !ok {
		return nil
	}

	var peerNodes []*peerNode
	seen := make(map[enode.ID]bool)
	for peerID := range peerIDs {
		peerNode, ok := g.crawledPeers.peerNodeByPid[peerID]
		if !ok {
			continue
		}
		if peerNode.isPinged && g.peerFilter(peerNode.node) {
			// Skip if we've already seen this enode ID
			if seen[peerNode.node.ID()] {
				continue
			}
			seen[peerNode.node.ID()] = true
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

func (g *GossipsubPeerCrawler) RemovePeerByPeerId(peerID peer.ID) {
	g.crawledPeers.removePeerByPeerId(peerID)
}

func (g *GossipsubPeerCrawler) RemoveTopic(topic string) {
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
					g.crawledPeers.removePeerByNodeId(node.ID())
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
			g.crawledPeers.removePeerByNodeId(node.ID())
			continue
		}

		topics, err := g.topicExtractor(ctx, node)
		if err != nil {
			log.WithError(err).WithField("node", node.ID()).Debug("Failed to extract topics, skipping")
			continue
		}

		shouldPing, err := g.crawledPeers.updateCrawledIfNewer(node, topics)
		if err != nil {
			log.WithError(err).WithField("node", node.ID()).Error("Failed to update crawled peers")
		}
		if !shouldPing {
			continue
		}
		select {
		case g.pingCh <- *node:
		case <-g.ctx.Done():
			return
		}
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
	peers := make([]*peerNode, 0, len(cp.peerNodeByPid))
	for _, p := range cp.peerNodeByPid {
		peers = append(peers, p)
	}
	cp.mu.RUnlock()

	for _, p := range peers {
		// Remove peers that no longer pass the filter
		if !g.peerFilter(p.node) {
			cp.removePeerByNodeId(p.node.ID())
			continue
		}

		// Re-extract topics; if the extractor errors or yields none, drop the peer.
		topics, err := g.topicExtractor(g.ctx, p.node)
		if err != nil || len(topics) == 0 {
			cp.removePeerByNodeId(p.node.ID())
		}
	}
}

// enodeToPeerID converts an enode record to a peer ID.
func enodeToPeerID(n *enode.Node) (peer.ID, error) {
	info, _, err := convertToAddrInfo(n)
	if err != nil {
		return "", fmt.Errorf("failed to convert enode to addr info: %w", err)
	}
	if info == nil {
		return "", errors.New("peer info is nil")
	}
	return info.ID, nil
}
