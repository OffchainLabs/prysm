package sync

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/gossipsubcrawler"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

type baseGossipsubTopicFamily struct {
	syncService *Service
	nse         params.NetworkScheduleEntry
	validator   wrappedVal
	handler     subHandler

	tf GossipsubTopicFamily

	mu            sync.Mutex
	subscriptions map[string]*pubsub.Subscription
}

func newBaseGossipsubTopicFamily(syncService *Service, nse params.NetworkScheduleEntry, validator wrappedVal,
	handler subHandler, tf GossipsubTopicFamily) *baseGossipsubTopicFamily {
	return &baseGossipsubTopicFamily{
		syncService:   syncService,
		nse:           nse,
		validator:     validator,
		handler:       handler,
		tf:            tf,
		subscriptions: make(map[string]*pubsub.Subscription),
	}
}

func (b *baseGossipsubTopicFamily) NetworkScheduleEntry() params.NetworkScheduleEntry {
	return b.nse
}

// idempotent for a topic
func (b *baseGossipsubTopicFamily) subscribeToTopics(topics []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, topic := range topics {
		log := log.WithField("topic", topic)
		s := b.syncService

		// Do not resubscribe to topics that we already have a subscription for.
		_, ok := b.subscriptions[topic]
		if ok {
			continue
		}

		if err := s.cfg.p2p.PubSub().RegisterTopicValidator(s.wrapAndReportValidation(topic, b.validator)); err != nil {
			log.WithError(err).Error("Could not register validator for topic")
			continue
		}

		sub, err := s.cfg.p2p.SubscribeToTopic(topic)
		if err != nil {
			// Any error subscribing to a PubSub topic would be the result of a misconfiguration of
			// libp2p PubSub library or a subscription request to a topic that fails to match the topic
			// subscription filter.
			log.WithError(err).Error("Could not subscribe topic")
			continue
		}

		// Pipeline decodes the incoming subscription data, runs the validation, and handles the
		// message.
		pipeline := func(msg *pubsub.Message) {
			ctx, cancel := context.WithTimeout(s.ctx, pubsubMessageTimeout)
			defer cancel()

			ctx, span := trace.StartSpan(ctx, "sync.pubsub")
			defer span.End()

			defer func() {
				if r := recover(); r != nil {
					tracing.AnnotateError(span, fmt.Errorf("panic occurred: %v", r))
					log.WithField("error", r).
						WithField("recoveredAt", "subscribeWithBase").
						WithField("stack", string(debug.Stack())).
						Error("Panic occurred")
				}
			}()

			span.SetAttributes(trace.StringAttribute("topic", topic))

			if msg.ValidatorData == nil {
				log.Error("Received nil message on pubsub")
				messageFailedProcessingCounter.WithLabelValues(topic).Inc()
				return
			}

			if err := b.handler(ctx, msg.ValidatorData.(proto.Message)); err != nil {
				tracing.AnnotateError(span, err)
				log.WithError(err).Error("Could not handle p2p pubsub")
				messageFailedProcessingCounter.WithLabelValues(topic).Inc()
				return
			}
		}

		// The main message loop for receiving incoming messages from this subscription.
		messageLoop := func() {
			for {
				msg, err := sub.Next(s.ctx)
				if err != nil {
					// This should only happen when the context is cancelled or subscription is cancelled.
					if !errors.Is(err, pubsub.ErrSubscriptionCancelled) { // Only log a warning on unexpected errors.
						log.WithError(err).Warn("Subscription next failed")
					}
					// Cancel subscription in the event of an error, as we are
					// now exiting topic event loop.
					sub.Cancel()
					return
				}

				if msg.ReceivedFrom == s.cfg.p2p.PeerID() {
					continue
				}

				go pipeline(msg)
			}
		}

		go messageLoop()
		log.WithField("topic", topic).Info("Subscribed to")
		b.subscriptions[topic] = sub
		s.subHandler.addTopic(topic, sub)
	}
}

func (b *baseGossipsubTopicFamily) unsubscribeAll() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for topic, sub := range b.subscriptions {
		b.cleanupSubscription(topic, sub)
	}

	b.subscriptions = make(map[string]*pubsub.Subscription)
}

func (b *baseGossipsubTopicFamily) removeUnwantedTopics(wantedTopics []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	topics := wantedTopics
	neededMap := make(map[string]bool, len(topics))
	for _, t := range topics {
		neededMap[t] = true
	}

	for topic, sub := range b.subscriptions {
		if !neededMap[topic] {
			b.cleanupSubscription(topic, sub)
		}
	}
}

func (b *baseGossipsubTopicFamily) cleanupSubscription(topic string, sub *pubsub.Subscription) {
	s := b.syncService
	log.WithField("topic", topic).Info("Unsubscribed from")
	if err := s.cfg.p2p.PubSub().UnregisterTopicValidator(topic); err != nil {
		log.WithError(err).Error("Could not unregister topic validator")
	}

	if sub != nil {
		sub.Cancel()
	}
	if err := s.cfg.p2p.LeaveTopic(topic); err != nil {
		log.WithError(err).Error("Unable to leave topic")
	}

	if crawler := s.cfg.p2p.Crawler(); crawler != nil {
		crawler.RemoveTopic(gossipsubcrawler.Topic(topic))
	}
	delete(b.subscriptions, topic)
	s.subHandler.removeTopic(topic)
}
