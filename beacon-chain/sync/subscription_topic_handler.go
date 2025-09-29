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
	// Check if this topic was previously reserved (nil value means reserved)
	prevSub, wasReserved := s.subTopics[topic]
	s.subTopics[topic] = sub
	digest, err := p2p.ExtractGossipDigest(topic)
	if err != nil {
		log.WithError(err).Error("Could not retrieve digest")
		return
	}
	// Only increment digest count if this is truly a new subscription:
	// - Topic didn't exist before (!wasReserved)
	// - Topic was reserved (nil) and now getting a real subscription
	if !wasReserved || prevSub == nil {
		s.digestMap[digest] += 1
	}
}

func (s *subTopicHandler) topicExists(topic string) bool {
	s.RLock()
	defer s.RUnlock()
	sub, ok := s.subTopics[topic]
	// Topic exists if it's in the map and has a real subscription (not just reserved)
	return ok && sub != nil
}

func (s *subTopicHandler) removeTopic(topic string) {
	s.Lock()
	defer s.Unlock()
	delete(s.subTopics, topic)
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

// tryReserveTopic atomically checks if a topic has an active subscription or reservation and reserves it if not.
// Returns true if the topic was successfully reserved, false if it already has an active subscription or reservation.
// If true is returned, the caller is responsible for calling addTopic with the actual subscription.
func (s *subTopicHandler) tryReserveTopic(topic string) bool {
	s.Lock()
	defer s.Unlock()
	_, exists := s.subTopics[topic]
	// Reject if topic already exists (either reserved with nil or has real subscription)
	if exists {
		return false
	}
	// Reserve the topic by adding a nil entry to prevent other goroutines from registering
	s.subTopics[topic] = nil
	return true
}

// cancelReservation removes a topic reservation if the subscription failed.
func (s *subTopicHandler) cancelReservation(topic string) {
	s.Lock()
	defer s.Unlock()
	if sub, exists := s.subTopics[topic]; exists && sub == nil {
		delete(s.subTopics, topic)
	}
}

