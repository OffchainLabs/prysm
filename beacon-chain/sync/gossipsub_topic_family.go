package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/ethereum/go-ethereum/p2p/enode"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

// wrappedVal represents a gossip validator which also returns an error along with the result.
type wrappedVal func(context.Context, peer.ID, *pubsub.Message) (pubsub.ValidationResult, error)

// subHandler represents handler for a given subscription.
type subHandler func(context.Context, proto.Message) error

// noopHandler is used for subscriptions that do not require anything to be done.
var noopHandler subHandler = func(ctx context.Context, msg proto.Message) error {
	return nil
}

type baseGossipsubTopicFamily struct {
	syncService    *Service
	protocolSuffix string
	nse            params.NetworkScheduleEntry
}

func (b *baseGossipsubTopicFamily) NetworkScheduleEntry() params.NetworkScheduleEntry {
	return b.nse
}

type GossipsubTopicFamily interface {
	Validator() wrappedVal
	Handler() subHandler
	NetworkScheduleEntry() params.NetworkScheduleEntry
	Subscribe()
	Unsubscribe()
}

type GossipsubTopicFamilyWithoutDynamicSubnets interface {
	GossipsubTopicFamily
	GetFullTopicString() string
}

type GossipsubTopicFamilyWithDynamicSubnets interface {
	GossipsubTopicFamily
	GetFullTopicString(subnet uint64) string
	GetSubnetsToJoin(slot primitives.Slot) map[uint64]bool
	GetSubnetsForBroadcast(slot primitives.Slot) map[uint64]bool
	GetTopicsForNode(node *enode.Node) ([]string, error)
}

type topicFamilyEntry struct {
	activationEpoch   primitives.Epoch
	deactivationEpoch *primitives.Epoch // optional; inactive at >= deactivationEpoch
	factory           func(s *Service, nse params.NetworkScheduleEntry) []GossipsubTopicFamily
}

var topicFamilySchedule = []topicFamilyEntry{
	// Genesis topic families
	{
		activationEpoch: params.BeaconConfig().GenesisEpoch,
		factory: func(s *Service, nse params.NetworkScheduleEntry) []GossipsubTopicFamily {
			return []GossipsubTopicFamily{
				NewBlockTopicFamily(s, nse),
				NewAggregateAndProofTopicFamily(s, nse),
				NewVoluntaryExitTopicFamily(s, nse),
				NewProposerSlashingTopicFamily(s, nse),
				NewAttesterSlashingTopicFamily(s, nse),
				NewAttestationTopicFamily(s, nse),
			}
		},
	},
	// Altair topic families
	{
		activationEpoch: params.BeaconConfig().AltairForkEpoch,
		factory: func(s *Service, nse params.NetworkScheduleEntry) []GossipsubTopicFamily {
			families := []GossipsubTopicFamily{
				NewSyncContributionAndProofTopicFamily(s, nse),
				NewSyncCommitteeTopicFamily(s, nse),
			}
			if features.Get().EnableLightClient {
				families = append(families,
					NewLightClientOptimisticUpdateTopicFamily(s, nse),
					NewLightClientFinalityUpdateTopicFamily(s, nse),
				)
			}
			return families
		},
	},
	// Capella topic families
	{
		activationEpoch: params.BeaconConfig().CapellaForkEpoch,
		factory: func(s *Service, nse params.NetworkScheduleEntry) []GossipsubTopicFamily {
			return []GossipsubTopicFamily{NewBlsToExecutionChangeTopicFamily(s, nse)}
		},
	},
	// Blob topic families (static per-subnet) in Deneb and Electra forks (removed in Fulu)
	{
		activationEpoch:   params.BeaconConfig().DenebForkEpoch,
		deactivationEpoch: func() *primitives.Epoch { e := params.BeaconConfig().ElectraForkEpoch; return &e }(),
		factory: func(s *Service, nse params.NetworkScheduleEntry) []GossipsubTopicFamily {
			count := params.BeaconConfig().BlobsidecarSubnetCount
			families := make([]GossipsubTopicFamily, 0, count)
			for i := uint64(0); i < count; i++ {
				families = append(families, NewBlobTopicFamily(s, nse, i))
			}
			return families
		},
	},
	{
		activationEpoch:   params.BeaconConfig().ElectraForkEpoch,
		deactivationEpoch: func() *primitives.Epoch { e := params.BeaconConfig().FuluForkEpoch; return &e }(),
		factory: func(s *Service, nse params.NetworkScheduleEntry) []GossipsubTopicFamily {
			count := params.BeaconConfig().BlobsidecarSubnetCountElectra
			families := make([]GossipsubTopicFamily, 0, count)
			for i := uint64(0); i < count; i++ {
				families = append(families, NewBlobTopicFamily(s, nse, i))
			}
			return families
		},
	},
	// Fulu data column topic family
	{
		activationEpoch: params.BeaconConfig().FuluForkEpoch,
		factory: func(s *Service, nse params.NetworkScheduleEntry) []GossipsubTopicFamily {
			return []GossipsubTopicFamily{NewDataColumnTopicFamily(s, nse)}
		},
	},
}

func TopicFamiliesForEpoch(epoch primitives.Epoch, s *Service, nse params.NetworkScheduleEntry) []GossipsubTopicFamily {
	var activeFamilies []GossipsubTopicFamily
	for _, entry := range topicFamilySchedule {
		if epoch < entry.activationEpoch {
			continue
		}
		if entry.deactivationEpoch != nil && epoch >= *entry.deactivationEpoch {
			continue
		}
		activeFamilies = append(activeFamilies, entry.factory(s, nse)...)
	}
	return activeFamilies
}
