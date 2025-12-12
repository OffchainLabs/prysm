package sync

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
)

var _ ShardedTopicFamily = (*BlobTopicFamily)(nil)

// BlobTopicFamily represents a static-subnet family instance for a specific blob subnet index.
type BlobTopicFamily struct {
	*baseTopicFamily
	subnetIndex uint64
}

func NewBlobTopicFamily(s *Service, nse params.NetworkScheduleEntry, subnetIndex uint64) *BlobTopicFamily {
	b := &BlobTopicFamily{
		subnetIndex: subnetIndex,
	}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateBlob, s.blobSubscriber, b)
	b.baseTopicFamily = base
	return b
}

func (b *BlobTopicFamily) Name() string {
	return fmt.Sprintf("BlobTopicFamily-%d", b.subnetIndex)
}

// Subscribe subscribes to the static subnet topic. Slot is ignored for this topic family.
func (b *BlobTopicFamily) Subscribe() {
	b.subscribeToTopics([]string{b.getFullTopicString()})
}

func (b *BlobTopicFamily) getFullTopicString() string {
	return p2p.BlobSubnetTopic(b.nse.ForkDigest, b.subnetIndex)
}
