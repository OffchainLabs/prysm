package p2p

import (
	"context"
	"fmt"
	"slices"
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
	peersByTopic    map[string]map[*peerNode]struct{}
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

func (cp *crawledPeers) updateCrawledIfNewer(node *enode.Node, topics []string) (bool, error) {
	if node == nil {
		return false, errors.New("node is nil")
	}

	return cp.updatePeer(node, topics)
}

func (cp *crawledPeers) updatePeer(node *enode.Node, topics []string) (bool, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	enodeID := node.ID()
	existingPNode, ok := cp.peerNodeByEnode[enodeID]

	if ok && existingPNode.node == nil {
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
			return false, fmt.Errorf("converting enode to peer ID: %w", err)
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
	peers, ok := cp.peersByTopic[topic]
	if !ok {
		return // Topic doesn't exist
	}

	// Remove the topic from each peer's topic list
	for pnode := range peers {
		delete(pnode.topics, topic)
		// remove the peer if it has no more topics left
		if len(pnode.topics) == 0 {
			cp.updateTopicsUnlocked(pnode, nil)
		}
	}

	// Remove the topic from byTopic map
	delete(cp.peersByTopic, topic)
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
		if peers, ok := cp.peersByTopic[t]; ok {
			delete(peers, pnode)
			if len(peers) == 0 {
				delete(cp.peersByTopic, t)
			}
		}
	}
	pnode.topics = nil // Clear topics to indicate removal.
}

func (cp *crawledPeers) removeOldTopicsFromPeer(pnode *peerNode, newTopics map[string]struct{}) {
	for oldTopic := range pnode.topics {
		if _, ok := newTopics[oldTopic]; !ok {
			if peers, ok := cp.peersByTopic[oldTopic]; ok {
				delete(peers, pnode)
				if len(peers) == 0 {
					delete(cp.peersByTopic, oldTopic)
				}
			}
		}
	}
}

func (cp *crawledPeers) addNewTopicsToPeer(pnode *peerNode, newTopics map[string]struct{}) {
	for newTopic := range newTopics {
		if _, ok := pnode.topics[newTopic]; !ok {
			if _, ok := cp.peersByTopic[newTopic]; !ok {
				cp.peersByTopic[newTopic] = make(map[*peerNode]struct{})
			}
			cp.peersByTopic[newTopic][pnode] = struct{}{}
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

func (cp *crawledPeers) getPeersForTopic(topic string, filter gossipsubcrawler.PeerFilterFunc) []*peerNode {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	peers, ok := cp.peersByTopic[topic]
	if !ok {
		return nil
	}

	var peerNodes []*peerNode
	seen := make(map[enode.ID]bool)
	for pnode := range peers {
		if pnode.node == nil {
			continue
		}
		if pnode.isPinged && filter(pnode.node) {
			// Skip if we've already seen this enode ID
			if seen[pnode.node.ID()] {
				continue
			}
			seen[pnode.node.ID()] = true
			peerNodes = append(peerNodes, pnode)
		}
	}
	return peerNodes
}

// GossipsubPeerCrawler discovers and maintains a registry of peers subscribed to gossipsub topics.
// It uses discv5 to find peers, extracts their topic subscriptions from ENR records, and verifies
// their reachability via ping. Only peers that have been successfully pinged are returned when
// querying for peers on a given topic. The crawler runs three background loops: one for discovery,
// one for ping verification, and one for periodic cleanup of stale or filtered-out peers.
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

	pingCh        chan enode.Node
	pingSemaphore *semaphore.Weighted

	wg   sync.WaitGroup
	once sync.Once
}

// cleanupInterval controls how frequently we sweep crawled peers and prune
// those that are no longer useful.
const cleanupInterval = 5 * time.Minute

// PeerScoreFunc calculates a reputation score for a given peer ID.
// Higher scores indicate more desirable peers. This function is used by PeersForTopic
// to sort returned peers in descending order of quality, allowing callers to prioritize
// connections to the most reliable peers.
type PeerScoreFunc func(peer.ID) float64

// NewGossipsubPeerCrawler creates a new crawler for discovering gossipsub peers.
// The crawler uses the provided discv5 listener to discover peers and tracks their
// topic subscriptions. Parameters:
//   - p2pSvc: The P2P service for network operations
//   - dv5: The discv5 listener used for peer discovery and ping verification
//   - crawlTimeout: Maximum duration for each crawl iteration
//   - crawlInterval: The duration between each crawl iteration
//   - maxConcurrentPings: Limits parallel ping operations to avoid overwhelming the network
//   - peerFilter: Determines which discovered peers should be tracked
//   - scorer: Calculates peer quality scores for sorting results
//
// Returns an error if any required parameter is nil or invalid.
func NewGossipsubPeerCrawler(
	p2pSvc *Service,
	dv5 ListenerRebooter,
	crawlTimeout time.Duration,
	crawlInterval time.Duration,
	maxConcurrentPings int64,
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
		ctx:           ctx,
		cancel:        cancel,
		crawlInterval: crawlInterval,
		crawlTimeout:  crawlTimeout,
		p2pSvc:        p2pSvc,
		dv5:           dv5,
		peerFilter:    peerFilter,
		scorer:        scorer,
	}
	g.pingCh = make(chan enode.Node, 4*maxConcurrentPings)
	g.pingSemaphore = semaphore.NewWeighted(maxConcurrentPings)
	g.crawledPeers = &crawledPeers{
		peerNodeByEnode: make(map[enode.ID]*peerNode),
		peerNodeByPid:   make(map[peer.ID]*peerNode),
		peersByTopic:    make(map[string]map[*peerNode]struct{}),
	}
	return g, nil
}

// PeersForTopic returns a list of enode records for peers subscribed to the given topic.
// Only peers that have been successfully pinged (verified as reachable) and pass the
// configured peer filter are included. Results are sorted in descending order by peer
// score, so higher-quality peers appear first. Returns nil if no peers are found for
// the topic. The returned slice should not be modified as it contains pointers to
// internal enode records.
func (g *GossipsubPeerCrawler) PeersForTopic(topic string) []*enode.Node {
	peerNodes := g.crawledPeers.getPeersForTopic(topic, g.peerFilter)

	slices.SortFunc(peerNodes, func(a, b *peerNode) int {
		scoreA := g.scorer(a.peerID)
		scoreB := g.scorer(b.peerID)
		if scoreA > scoreB {
			return -1
		}
		if scoreA < scoreB {
			return 1
		}
		return 0
	})

	nodes := make([]*enode.Node, 0, len(peerNodes))
	for _, pn := range peerNodes {
		nodes = append(nodes, pn.node)
	}

	return nodes
}

// RemovePeerByPeerId removes a peer from the crawler's registry by their libp2p peer ID.
// This also removes the peer from all topic subscriptions they were associated with.
// If the peer is not found, this operation is a no-op.
func (g *GossipsubPeerCrawler) RemovePeerByPeerId(peerID peer.ID) {
	g.crawledPeers.removePeerByPeerId(peerID)
}

// RemoveTopic removes a topic and all its peer associations from the crawler.
// Peers that were only subscribed to this topic are completely removed from the registry.
// Peers subscribed to other topics remain tracked for those topics.
// If the topic does not exist, this operation is a no-op.
func (g *GossipsubPeerCrawler) RemoveTopic(topic string) {
	g.crawledPeers.removeTopic(topic)
}

// Start begins the crawler's background operations. It launches three goroutines:
// a crawl loop that periodically discovers new peers via discv5, a ping loop that
// verifies peer reachability, and a cleanup loop that removes stale or filtered peers.
// The provided TopicExtractor is used to determine which gossipsub topics each
// discovered peer subscribes to. Start is idempotent; subsequent calls after the
// first are no-ops. Returns an error if the topic extractor is nil.
func (g *GossipsubPeerCrawler) Start(te gossipsubcrawler.TopicExtractor) error {
	if te == nil {
		return errors.New("topic extractor is nil")
	}
	g.once.Do(func() {
		g.topicExtractor = te
		g.wg.Go(g.crawlLoop)
		g.wg.Go(g.pingLoop)
		g.wg.Go(g.cleanupLoop)
	})

	return nil
}

// Stop terminates all background crawler operations and waits for them to complete.
// It cancels the crawler's context, which signals all goroutines to exit, then blocks
// until all goroutines have finished. After Stop returns, the crawler will no longer
// discover new peers or process pings. Stop is safe to call multiple times.
func (g *GossipsubPeerCrawler) Stop() {
	g.cancel()
	g.wg.Wait()
}

func (g *GossipsubPeerCrawler) pingLoop() {
	for {
		select {
		case node := <-g.pingCh:
			if err := g.pingSemaphore.Acquire(g.ctx, 1); err != nil {
				return
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
	for {
		g.crawl()
		select {
		case <-time.After(g.crawlInterval):
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
		return "", fmt.Errorf("converting enode to addr info: %w", err)
	}
	if info == nil {
		return "", errors.New("peer info is nil")
	}
	return info.ID, nil
}
