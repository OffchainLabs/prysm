package sync

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/config/params"
)

var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*BlobTopicFamily)(nil)

// BlobTopicFamily represents a static-subnet family instance for a specific blob subnet index.
type BlobTopicFamily struct {
	baseGossipsubTopicFamily
	subnetIndex uint64
}

func NewBlobTopicFamily(s *Service, nse params.NetworkScheduleEntry, subnetIndex uint64) *BlobTopicFamily {
	return &BlobTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix(),
		},
		subnetIndex,
	}
}

func (b *BlobTopicFamily) Validator() wrappedVal {
	return b.syncService.validateBlob
}

func (b *BlobTopicFamily) Handler() subHandler {
	return b.syncService.blobSubscriber
}

func (b *BlobTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.BlobSubnetTopicFormat, b.nse.ForkDigest, b.subnetIndex) + b.protocolSuffix
}

func (b *BlobTopicFamily) Subscribe() {
	b.syncService.subscribe(b.GetFullTopicString(), b.Validator(), b.Handler())
}

func (b *BlobTopicFamily) Unsubscribe() {
	b.syncService.unSubscribeFromTopic(b.GetFullTopicString())
}
