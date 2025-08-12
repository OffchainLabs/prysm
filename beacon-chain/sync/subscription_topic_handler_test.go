package sync

import (
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func TestSubTopicHandler_CRUD(t *testing.T) {
	h := newSubTopicHandler()
	// Non-existent topic
	assert.Equal(t, false, h.topicExists("junk"))
	assert.Equal(t, false, h.digestExists([4]byte{}))

	clock := startup.NewClock(time.Now(), [32]byte{})
	digest := params.ForkDigest(clock.CurrentEpoch())
	enc := encoder.SszNetworkEncoder{}

	// Valid topic added in.
	topic := fmt.Sprintf(p2p.BlockSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.addTopic(topic, new(pubsub.Subscription))
	assert.Equal(t, true, h.topicExists(topic))
	assert.Equal(t, true, h.digestExists(digest))
	assert.Equal(t, 1, len(h.allTopics()))

	h.removeTopic(topic)
	assert.Equal(t, false, h.topicExists(topic))
	assert.Equal(t, false, h.digestExists(digest))
	assert.Equal(t, 0, len(h.allTopics()))

	h = newSubTopicHandler()
	// Multiple Topics added in.
	topic = fmt.Sprintf(p2p.BlockSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.addTopic(topic, new(pubsub.Subscription))
	assert.Equal(t, true, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.ExitSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.addTopic(topic, new(pubsub.Subscription))
	assert.Equal(t, true, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.ProposerSlashingSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.addTopic(topic, new(pubsub.Subscription))
	assert.Equal(t, true, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.AttesterSlashingSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.addTopic(topic, new(pubsub.Subscription))
	assert.Equal(t, true, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.AggregateAndProofSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.addTopic(topic, new(pubsub.Subscription))
	assert.Equal(t, true, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.SyncContributionAndProofSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.addTopic(topic, new(pubsub.Subscription))
	assert.Equal(t, true, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.BlsToExecutionChangeSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.addTopic(topic, new(pubsub.Subscription))
	assert.Equal(t, true, h.topicExists(topic))

	assert.Equal(t, 7, len(h.allTopics()))

	// Remove multiple topics
	topic = fmt.Sprintf(p2p.AttesterSlashingSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.removeTopic(topic)
	assert.Equal(t, false, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.ExitSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.removeTopic(topic)
	assert.Equal(t, false, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.ProposerSlashingSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.removeTopic(topic)
	assert.Equal(t, false, h.topicExists(topic))

	assert.Equal(t, true, h.digestExists(digest))
	assert.Equal(t, 4, len(h.allTopics()))

	// Remove remaining topics.
	topic = fmt.Sprintf(p2p.BlockSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.removeTopic(topic)
	assert.Equal(t, false, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.AggregateAndProofSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.removeTopic(topic)
	assert.Equal(t, false, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.SyncContributionAndProofSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.removeTopic(topic)
	assert.Equal(t, false, h.topicExists(topic))

	topic = fmt.Sprintf(p2p.BlsToExecutionChangeSubnetTopicFormat, digest) + enc.ProtocolSuffix()
	h.removeTopic(topic)
	assert.Equal(t, false, h.topicExists(topic))

	assert.Equal(t, false, h.digestExists(digest))
	assert.Equal(t, 0, len(h.allTopics()))
}
