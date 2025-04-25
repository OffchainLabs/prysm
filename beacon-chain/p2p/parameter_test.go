package p2p

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func TestOverlayParameters(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	pms := pubsubGossipParam()
	assert.Equal(t, gossipSubD, pms.D, "gossipSubD")
	assert.Equal(t, gossipSubDlo, pms.Dlo, "gossipSubDlo")
	assert.Equal(t, gossipSubDhi, pms.Dhi, "gossipSubDhi")
}

func TestGossipParameters(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	setPubSubParameters()
	pms := pubsubGossipParam()
	assert.Equal(t, gossipSubMcacheLen, pms.HistoryLength, "gossipSubMcacheLen")
	assert.Equal(t, gossipSubMcacheGossip, pms.HistoryGossip, "gossipSubMcacheGossip")
	assert.Equal(t, gossipSubSeenTTL, int(pubsub.TimeCacheDuration.Seconds()), "gossipSubSeenTtl")
}

func TestFanoutParameters(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	pms := pubsubGossipParam()
	if pms.FanoutTTL != gossipSubFanoutTTL {
		t.Errorf("gossipSubFanoutTTL, wanted: %d, got: %d", gossipSubFanoutTTL, pms.FanoutTTL)
	}
}

func TestHeartbeatParameters(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	pms := pubsubGossipParam()
	if pms.HeartbeatInterval != gossipSubHeartbeatInterval {
		t.Errorf("gossipSubHeartbeatInterval, wanted: %d, got: %d", gossipSubHeartbeatInterval, pms.HeartbeatInterval)
	}
}

func TestMiscParameters(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	setPubSubParameters()
	assert.Equal(t, rSubD, 8, "rSubD")
}
