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
var logStatsInterval = 5 * time.Minute

type peerNode struct {
	id     enode.ID
	node   *enode.Node
	peerID peer.ID
	topics map[string]struct{}
}

type crawledPeers struct {
	mu       sync.RWMutex
	byEnode  map[enode.ID]*peerNode
	byPeerId map[peer.ID]*peerNode
	byTopic  map[string]map[peer.ID]struct{}
}

func newCrawledPeers() *crawledPeers {
	return &crawledPeers{
		byEnode:  make(map[enode.ID]*peerNode),
		byPeerId: make(map[peer.ID]*peerNode),
		byTopic:  make(map[string]map[peer.ID]struct{}),
	}
}

// PeersForTopic returns a slice of peers known to be associated with the given topic.
func (p *crawledPeers) PeersForTopic(topic string) []*enode.Node {
	p.mu.RLock()
	defer p.mu.RUnlock()

	peers, ok := p.byTopic[topic]
	if !ok {
		return nil
	}
	var result []*enode.Node
	for pid := range peers {
		if pNode, ok := p.byPeerId[pid]; ok {
			result = append(result, pNode.node)
		}
	}
	return result
}

// RemoveTopic removes all peer associations for a given topic.
func (p *crawledPeers) RemoveTopic(topic string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	peers, ok := p.byTopic[topic]
	if !ok {
		return
	}
	delete(p.byTopic, topic)

	for pid := range peers {
		pNode, ok := p.byPeerId[pid]
		if !ok {
			continue
		}
		// Remove the topic from the peer's topic set.
		delete(pNode.topics, topic)

		// If the peer is no longer in any topics, remove it entirely.
		if len(pNode.topics) == 0 {
			p.removePeerUnlocked(pNode.id)
		}
	}
}

// RemovePeerID removes a peer using its libp2p peer.ID.
func (p *crawledPeers) RemovePeerID(pid peer.ID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pNode, ok := p.byPeerId[pid]
	if ok {
		p.removePeerUnlocked(pNode.id)
	}
}

func (p *crawledPeers) removePeer(enodeID enode.ID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removePeerUnlocked(enodeID)
}

func (p *crawledPeers) removePeerUnlocked(enodeID enode.ID) {
	pNode, ok := p.byEnode[enodeID]
	if !ok {
		return
	}
	delete(p.byPeerId, pNode.peerID)
	delete(p.byEnode, enodeID)

	for topic := range pNode.topics {
		if peers, ok := p.byTopic[topic]; ok {
			delete(peers, pNode.peerID)
			if len(peers) == 0 {
				delete(p.byTopic, topic)
			}
		}
	}
}

// setPeer sets or updates a peer's topics and node record. If topics is empty, removes the peer.
func (p *crawledPeers) setPeer(node *enode.Node, topics []string) {
	// If topics empty, remove immediately without needing peer conversion.
	if len(topics) == 0 {
		p.removePeer(node.ID())
		return
	}

	peerData, _, err := convertToAddrInfo(node)
	if err != nil || peerData == nil {
		log.WithError(err).WithField("node", node.ID()).Debug("Failed to convert node to peer address info")
		return
	}

	// Build new topic set
	newSet := make(map[string]struct{}, len(topics))
	for _, t := range topics {
		newSet[t] = struct{}{}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.shouldUpdateUnlocked(node) {
		return
	}

	enodeID := node.ID()
	pNode, exists := p.byEnode[enodeID]
	if exists {
		// Diff remove: topics present before but not now
		for old := range pNode.topics {
			if _, ok := newSet[old]; !ok {
				if peers, ok := p.byTopic[old]; ok {
					delete(peers, pNode.peerID)
					if len(peers) == 0 {
						delete(p.byTopic, old)
					}
				}
				delete(pNode.topics, old)
			}
		}
		// Diff add: topics present now but not before
		for t := range newSet {
			if _, ok := pNode.topics[t]; !ok {
				if _, ok := p.byTopic[t]; !ok {
					p.byTopic[t] = make(map[peer.ID]struct{})
				}
				p.byTopic[t][pNode.peerID] = struct{}{}
				pNode.topics[t] = struct{}{}
			}
		}
		// Update node reference
		pNode.node = node
		// If peer ID changed, update index
		if pNode.peerID != peerData.ID {
			delete(p.byPeerId, pNode.peerID)
			pNode.peerID = peerData.ID
			p.byPeerId[peerData.ID] = pNode
		}
	} else {
		// Create new entry
		newPeer := &peerNode{
			id:     enodeID,
			node:   node,
			peerID: peerData.ID,
			topics: newSet,
		}
		p.byEnode[enodeID] = newPeer
		p.byPeerId[peerData.ID] = newPeer
		for t := range newSet {
			if _, ok := p.byTopic[t]; !ok {
				p.byTopic[t] = make(map[peer.ID]struct{})
			}
			p.byTopic[t][peerData.ID] = struct{}{}
		}
	}
}

// shouldUpdateUnlocked checks if we should update a peer's information.
// It returns true if the peer is new or the new node record has a higher sequence number.
func (p *crawledPeers) shouldUpdateUnlocked(node *enode.Node) bool {
	enodeID := node.ID()
	existingPNode, ok := p.byEnode[enodeID]
	if !ok {
		return true // Peer is new.
	}
	// only update if new record is newer.
	return node.Seq() > existingPNode.node.Seq()
}

// Crawler periodically crawls the discovery network for peers and maintains a
// mapping of topics to the peers that support them and their ENRs.
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

	peers *crawledPeers
}

// newCrawler creates a new peer crawler.
func newCrawler(dv5 Listener, service *Service, interval time.Duration) (*Crawler, error) {
	if dv5 == nil {
		return nil, errors.New("discovery client is required")
	}
	if service == nil {
		return nil, errors.New("p2p service reference is required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Crawler{
		ctx:           ctx,
		cancel:        cancel,
		dv5:           dv5,
		service:       service,
		crawlInterval: interval,
		crawlTimeout:  crawlTimeout,
		peers:         newCrawledPeers(),
	}, nil
}

// Start begins the periodic crawling of the network.
func (c *Crawler) Start(topicExtractor gossipsubcrawler.TopicExtractor) error {
	if topicExtractor == nil {
		return errors.New("topic extractor cannot be nil")
	}
	return sync.OnceValue(func() error {
		return c.spawn(topicExtractor)
	})()
}

func (c *Crawler) spawn(topicExtractor gossipsubcrawler.TopicExtractor) error {
	c.topicExtractor = topicExtractor
	log.Info("Starting peer crawler")
	c.wg.Go(c.run)
	c.wg.Go(c.logStats)
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

// logStats logs statistics about topics and peers periodically
func (c *Crawler) logStats() {
	ticker := time.NewTicker(logStatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.peers.mu.RLock()
			totalPeers := len(c.peers.byEnode)
			totalTopics := len(c.peers.byTopic)

			fields := logrus.Fields{
				"totalPeers":  totalPeers,
				"totalTopics": totalTopics,
			}

			// Log each topic and its peer count
			for topic, peers := range c.peers.byTopic {
				fields[topic] = len(peers)
			}
			c.peers.mu.RUnlock()

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
			c.peers.setPeer(node, nil)
			continue
		}

		topics, err := c.topicExtractor(ctx, node)
		if err != nil {
			// It's okay for the callback to error, just means we can't process this node.
			log.WithError(err).WithField("node", node.ID()).Debug("Failed to extract topics from node")
			continue
		}
		if len(topics) == 0 {
			// If no topics are returned, remove it if tracked.
			c.peers.setPeer(node, nil)
			continue
		}
		c.peers.setPeer(node, topics)
	}
}

// PeersForTopic delegates to crawledPeers.
func (c *Crawler) PeersForTopic(topic string) []*enode.Node { return c.peers.PeersForTopic(topic) }

// RemoveTopic delegates to crawledPeers.
func (c *Crawler) RemoveTopic(topic string) { c.peers.RemoveTopic(topic) }

// removePeer helpers moved into crawledPeers

// RemovePeerID delegates to crawledPeers
func (c *Crawler) RemovePeerID(pid peer.ID) { c.peers.RemovePeerID(pid) }
