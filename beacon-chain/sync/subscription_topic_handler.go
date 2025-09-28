package sync

import (
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// This is a subscription topic handler that is used to handle basic
// CRUD operations on the topic map. All operations are thread safe
// so they can be called from multiple routines.
type subTopicHandler struct {
	sync.RWMutex
	subTopics map[string]*pubsub.Subscription
	digestMap map[[4]byte]int
}

func newSubTopicHandler() *subTopicHandler {
	return &subTopicHandler{
		subTopics: map[string]*pubsub.Subscription{},
		digestMap: map[[4]byte]int{},
	}
}

func (s *subTopicHandler) addTopic(topic string, sub *pubsub.Subscription) {
	s.Lock()
	defer s.Unlock()

	// Check if this is updating a reserved entry (nil subscription)
	existingSub, exists := s.subTopics[topic]
	wasReserved := exists && existingSub == nil

	s.subTopics[topic] = sub
	digest, err := p2p.ExtractGossipDigest(topic)
	if err != nil {
		log.WithError(err).Error("Could not retrieve digest")
		return
	}

	// Only increment digest count if this is a new topic (not just updating a reservation)
	if !wasReserved {
		s.digestMap[digest] += 1
	}
}

func (s *subTopicHandler) topicExists(topic string) bool {
	s.RLock()
	defer s.RUnlock()
	_, ok := s.subTopics[topic]
	return ok
}

// tryReserveTopic atomically checks if a topic exists and reserves it if not.
// Returns true if the topic was successfully reserved (didn't exist before),
// false if the topic already exists or is reserved.
// This prevents the race condition where multiple goroutines check topicExists()
// simultaneously and both proceed to subscribe.
func (s *subTopicHandler) tryReserveTopic(topic string) bool {
	s.Lock()
	defer s.Unlock()

	// Check if topic already exists or is reserved
	if _, exists := s.subTopics[topic]; exists {
		return false
	}

	// Reserve the topic with a nil placeholder
	// This will be updated with the actual subscription later
	s.subTopics[topic] = nil
	return true
}

func (s *subTopicHandler) removeTopic(topic string) {
	s.Lock()
	defer s.Unlock()

	// Check if topic exists and whether it was just a reservation (nil)
	existingSub, exists := s.subTopics[topic]
	if !exists {
		return
	}
	wasReserved := existingSub == nil

	delete(s.subTopics, topic)

	// Only decrement digest count if this wasn't just a reservation
	if !wasReserved {
		digest, err := p2p.ExtractGossipDigest(topic)
		if err != nil {
			log.WithError(err).Error("Could not retrieve digest")
			return
		}
		currAmt, ok := s.digestMap[digest]
		// Should never be possible, is a
		// defensive check.
		if !ok || currAmt <= 0 {
			delete(s.digestMap, digest)
			return
		}
		s.digestMap[digest] -= 1
		if s.digestMap[digest] == 0 {
			delete(s.digestMap, digest)
		}
	}
}

func (s *subTopicHandler) digestExists(digest [4]byte) bool {
	s.RLock()
	defer s.RUnlock()

	count, ok := s.digestMap[digest]
	return ok && count > 0
}

func (s *subTopicHandler) allTopics() []string {
	s.RLock()
	defer s.RUnlock()
	var topics []string
	for t := range s.subTopics {
		copiedTopic := t
		topics = append(topics, copiedTopic)
	}
	return topics
}

func (s *subTopicHandler) subForTopic(topic string) *pubsub.Subscription {
	s.RLock()
	defer s.RUnlock()
	return s.subTopics[topic]
}
