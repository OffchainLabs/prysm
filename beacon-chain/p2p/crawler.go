// crawler.go implements a discovery-based peer crawler to maintain a cache
// of peers that support specific gossipsub topics.
package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/gossipsubcrawler"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var crawlTimeout = 20 * time.Second
var cleanupInterval = 5 * time.Minute

type peerNode struct {
	id     enode.ID
	node   *enode.Node
	peerID peer.ID
	topics []string
}

// Crawler periodically crawls the discovery network for peers and maintains a
// mapping of topics to the peers that support them and their ENRs.
// TODO: Prevent unbounded growth of the topicToPeers map.
type Crawler struct {
	ctx    context.Context
	cancel context.CancelFunc

	// Discovery interface for finding peers
	dv5 Listener

	topicExtractor gossipsubcrawler.TopicExtractor

	// Reference to the p2p service for peer filtering and scoring
	service *Service

	crawlInterval time.Duration
	crawlTimeout  time.Duration

	wg sync.WaitGroup

	mu              sync.RWMutex
	started         bool                             // track if crawler has been started
	peerNodes       map[enode.ID]*peerNode           // enode.ID -> peerNode record
	topicToPeers    map[string]map[enode.ID]struct{} // topic -> set of enode.IDs
	peerIDToEnodeID map[peer.ID]enode.ID             // libp2p peer.ID -> discv5 enode.ID
}

// newCrawler creates a new peer crawler.
func newCrawler(ctx context.Context, dv5 Listener, service *Service, interval time.Duration) (*Crawler, error) {
	if dv5 == nil {
		return nil, errors.New("discovery client is required")
	}
	if service == nil {
		return nil, errors.New("p2p service reference is required")
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Crawler{
		ctx:             ctx,
		cancel:          cancel,
		dv5:             dv5,
		service:         service,
		crawlInterval:   interval,
		crawlTimeout:    crawlTimeout,
		peerNodes:       make(map[enode.ID]*peerNode),
		topicToPeers:    make(map[string]map[enode.ID]struct{}),
		peerIDToEnodeID: make(map[peer.ID]enode.ID),
	}, nil
}

// Start begins the periodic crawling of the network.
func (c *Crawler) Start(topicExtractor gossipsubcrawler.TopicExtractor) error {
	if topicExtractor == nil {
		return errors.New("topic extractor cannot be nil")
	}
	c.topicExtractor = topicExtractor

	// Mark crawler as started
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return errors.New("crawler already started")
	}
	c.started = true
	c.mu.Unlock()

	log.Info("Starting peer crawler")
	c.wg.Add(2)
	go c.run()
	go c.logStats()
	return nil
}

// Stop terminates the crawler's background processing.
func (c *Crawler) Stop() {
	log.Info("Stopping peer crawler")
	c.cancel()
	c.wg.Wait()

	log.Info("Peer crawler stopped")
}

func (c *Crawler) run() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.crawlInterval)
	defer ticker.Stop()

	// Perform an initial crawl immediately.
	c.crawl()
	for {
		select {
		case <-ticker.C:
			c.crawl()
		case <-c.ctx.Done():
			return
		}
	}
}

// logStats logs statistics about topics and peers every 5 minutes
func (c *Crawler) logStats() {
	defer c.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.RLock()
			totalPeers := len(c.peerNodes)
			totalTopics := len(c.topicToPeers)

			fields := logrus.Fields{
				"totalPeers":  totalPeers,
				"totalTopics": totalTopics,
			}

			// Log each topic and its peer count
			for topic, peers := range c.topicToPeers {
				fields[topic] = len(peers)
			}
			c.mu.RUnlock()

			log.WithFields(fields).Debug("Crawler topic statistics")
		case <-c.ctx.Done():
			return
		}
	}
}

// crawl performs a single crawl of the discovery network.
func (c *Crawler) crawl() {
	ctx, cancel := context.WithTimeout(c.ctx, c.crawlTimeout)
	defer cancel()

	iterator := c.dv5.RandomNodes()
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

		if !c.service.filterPeer(node) {
			c.mu.Lock()
			c.removePeerUnlocked(node.ID())
			c.mu.Unlock()
			continue
		}

		topics, err := c.topicExtractor(ctx, node)
		if err != nil {
			// It's okay for the callback to error, just means we can't process this node.
			log.WithError(err).WithField("node", node.ID()).Debug("Failed to extract topics from node")
			continue
		}
		if len(topics) == 0 {
			// If no topics are returned, we don't track the peer.
			// We should also remove it if it's already tracked.
			c.mu.Lock()
			c.removePeerUnlocked(node.ID())
			c.mu.Unlock()
			continue
		}
		c.addOrUpdatePeer(node, topics)
	}
}

// PeersForTopic returns a slice of peers known to be associated with the given topic.
func (c *Crawler) PeersForTopic(topic string) []*enode.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()

	peers, ok := c.topicToPeers[topic]
	if !ok {
		return nil
	}
	var result []*enode.Node
	for enodeID := range peers {
		if pNode, ok := c.peerNodes[enodeID]; ok {
			result = append(result, pNode.node)
		}
	}
	return result
}

// RemoveTopic removes all peer associations for a given topic. This is useful when
// a beacon node unsubscribes from a topic and no longer needs to track its peers.
func (c *Crawler) RemoveTopic(topic string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	peers, ok := c.topicToPeers[topic]
	if !ok {
		return
	}
	delete(c.topicToPeers, topic)

	for enodeID := range peers {
		pNode, ok := c.peerNodes[enodeID]
		if !ok {
			continue
		}
		// Remove the topic from the peer's topic list.
		var newTopics []string
		for _, t := range pNode.topics {
			if t != topic {
				newTopics = append(newTopics, t)
			}
		}
		pNode.topics = newTopics

		// If the peer is no longer in any topics, remove it entirely.
		if len(pNode.topics) == 0 {
			c.removePeerUnlocked(enodeID)
		}
	}
}

func (c *Crawler) removePeerUnlocked(enodeID enode.ID) {
	pNode, ok := c.peerNodes[enodeID]
	if !ok {
		return
	}
	delete(c.peerIDToEnodeID, pNode.peerID)
	delete(c.peerNodes, enodeID)

	for _, topic := range pNode.topics {
		if peers, ok := c.topicToPeers[topic]; ok {
			delete(peers, enodeID)
			if len(peers) == 0 {
				delete(c.topicToPeers, topic)
			}
		}
	}
}

// RemovePeerByPeerID removes a peer using its libp2p peer.ID.
func (c *Crawler) RemovePeerByPeerID(pid peer.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	enodeID, ok := c.peerIDToEnodeID[pid]

	if ok {
		c.removePeerUnlocked(enodeID)
	}
}

// addOrUpdatePeer adds a new peer's topic associations to the crawler's cache or
// updates the existing entry if the new node record has a higher sequence number.
// It also cleans up the peer's old topic associations if they have changed.
func (c *Crawler) addOrUpdatePeer(node *enode.Node, newTopics []string) {
	peerData, _, err := convertToAddrInfo(node)
	if err != nil || peerData == nil {
		log.WithError(err).WithField("node", node.ID()).Debug("Failed to convert node to peer address info")
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.shouldUpdate(node) {
		return
	}

	enodeID := node.ID()
	pNode, exists := c.peerNodes[enodeID]

	// If the peer exists, remove it from any topics it no longer supports.
	if exists {
		c.removeOldTopics(pNode)
	}

	// Add the peer to all its new topics.
	c.addNewTopics(enodeID, newTopics)

	// Update the peer's node record and topic list.
	if exists {
		pNode.node = node
		pNode.topics = newTopics
	} else {
		// Or create a new entry if it's the first time we see it.
		c.peerNodes[enodeID] = &peerNode{
			id:     enodeID,
			node:   node,
			peerID: peerData.ID,
			topics: newTopics,
		}
		c.peerIDToEnodeID[peerData.ID] = enodeID
	}
}

// shouldUpdate checks if the crawler should update a peer's information.
// It returns true if the peer is new or if the new node record has a higher
// sequence number than the existing one.
// This method assumes the crawler's mutex is already locked.
func (c *Crawler) shouldUpdate(node *enode.Node) bool {
	enodeID := node.ID()
	existingPNode, ok := c.peerNodes[enodeID]
	if !ok {
		return true // Peer is new.
	}
	// only update if new record is newer.
	return node.Seq() > existingPNode.node.Seq()
}

// removeOldTopics removes a peer from all topics it participates in.
// This method assumes the crawler's mutex is already locked.
func (c *Crawler) removeOldTopics(pNode *peerNode) {
	for _, oldTopic := range pNode.topics {
		if peers, ok := c.topicToPeers[oldTopic]; ok {
			delete(peers, pNode.id)
			if len(peers) == 0 {
				delete(c.topicToPeers, oldTopic)
			}
		}
	}
}

// addNewTopics adds a peer to its new topics in the crawler's cache.
// This method assumes the crawler's mutex is already locked.
func (c *Crawler) addNewTopics(enodeID enode.ID, newTopics []string) {
	for _, topic := range newTopics {
		if _, ok := c.topicToPeers[topic]; !ok {
			c.topicToPeers[topic] = make(map[enode.ID]struct{})
		}
		c.topicToPeers[topic][enodeID] = struct{}{}
	}
}
