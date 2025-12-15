package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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

type TopicFamily interface {
	Name() string
	NetworkScheduleEntry() params.NetworkScheduleEntry
	UnsubscribeAll()
}

type ShardedTopicFamily interface {
	TopicFamily
	Subscribe()
}

type DynamicShardedTopicFamily interface {
	TopicFamily

	TopicsWithMinPeerCount(slot primitives.Slot) map[string]int

	TopicsToSubscribeForSlot(slot primitives.Slot) []string

	ExtractTopicsForNode(node *enode.Node) ([]string, error)

	SubscribeForSlot(slot primitives.Slot)

	UnsubscribeForSlot(slot primitives.Slot)
}

type topicFamilyEntry struct {
	activationEpoch   primitives.Epoch
	deactivationEpoch primitives.Epoch
	factory           func(s *Service, nse params.NetworkScheduleEntry) []TopicFamily
}

func topicFamilySchedule() []topicFamilyEntry {
	cfg := params.BeaconConfig()
	return []topicFamilyEntry{
		// Genesis topic families
		{
			activationEpoch:   cfg.GenesisEpoch,
			deactivationEpoch: cfg.FarFutureEpoch,
			factory: func(s *Service, nse params.NetworkScheduleEntry) []TopicFamily {
				return []TopicFamily{
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
			activationEpoch:   cfg.AltairForkEpoch,
			deactivationEpoch: cfg.FarFutureEpoch,
			factory: func(s *Service, nse params.NetworkScheduleEntry) []TopicFamily {
				families := []TopicFamily{
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
			activationEpoch:   cfg.CapellaForkEpoch,
			deactivationEpoch: cfg.FarFutureEpoch,
			factory: func(s *Service, nse params.NetworkScheduleEntry) []TopicFamily {
				return []TopicFamily{NewBlsToExecutionChangeTopicFamily(s, nse)}
			},
		},
		// Blob topic families (static per-subnet) in Deneb and Electra forks (removed in Fulu)
		{
			activationEpoch:   cfg.DenebForkEpoch,
			deactivationEpoch: cfg.ElectraForkEpoch,
			factory: func(s *Service, nse params.NetworkScheduleEntry) []TopicFamily {
				count := cfg.BlobsidecarSubnetCount
				families := make([]TopicFamily, 0, count)
				for i := range count {
					families = append(families, NewBlobTopicFamily(s, nse, i))
				}
				return families
			},
		},
		{
			activationEpoch:   cfg.ElectraForkEpoch,
			deactivationEpoch: cfg.FuluForkEpoch,
			factory: func(s *Service, nse params.NetworkScheduleEntry) []TopicFamily {
				count := cfg.BlobsidecarSubnetCountElectra
				families := make([]TopicFamily, 0, count)
				for i := range count {
					families = append(families, NewBlobTopicFamily(s, nse, i))
				}
				return families
			},
		},
		// Fulu data column topic family
		{
			activationEpoch:   cfg.FuluForkEpoch,
			deactivationEpoch: cfg.FarFutureEpoch,
			factory: func(s *Service, nse params.NetworkScheduleEntry) []TopicFamily {
				return []TopicFamily{NewDataColumnTopicFamily(s, nse)}
			},
		},
	}
}

func TopicFamiliesForEpoch(epoch primitives.Epoch, s *Service, nse params.NetworkScheduleEntry) []TopicFamily {
	var activeFamilies []TopicFamily
	for _, entry := range topicFamilySchedule() {
		if epoch < entry.activationEpoch {
			continue
		}
		if epoch >= entry.deactivationEpoch {
			continue
		}
		activeFamilies = append(activeFamilies, entry.factory(s, nse)...)
	}
	return activeFamilies
}
