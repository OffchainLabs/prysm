package p2p

import (
	"reflect"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// gossipTopicMappings represent the protocol ID to protobuf message type map for easy
// lookup.
var gossipTopicMappings = map[string]func() proto.Message{
	BlockSubnetTopicFormat:                    func() proto.Message { return &ethpb.SignedBeaconBlock{} },
	AttestationSubnetTopicFormat:              func() proto.Message { return &ethpb.Attestation{} },
	ExitSubnetTopicFormat:                     func() proto.Message { return &ethpb.SignedVoluntaryExit{} },
	ProposerSlashingSubnetTopicFormat:         func() proto.Message { return &ethpb.ProposerSlashing{} },
	AttesterSlashingSubnetTopicFormat:         func() proto.Message { return &ethpb.AttesterSlashing{} },
	AggregateAndProofSubnetTopicFormat:        func() proto.Message { return &ethpb.SignedAggregateAttestationAndProof{} },
	SyncContributionAndProofSubnetTopicFormat: func() proto.Message { return &ethpb.SignedContributionAndProof{} },
	SyncCommitteeSubnetTopicFormat:            func() proto.Message { return &ethpb.SyncCommitteeMessage{} },
	BlsToExecutionChangeSubnetTopicFormat:     func() proto.Message { return &ethpb.SignedBLSToExecutionChange{} },
	BlobSubnetTopicFormat:                     func() proto.Message { return &ethpb.BlobSidecar{} },
	LightClientOptimisticUpdateTopicFormat:    func() proto.Message { return &ethpb.LightClientOptimisticUpdateAltair{} },
	LightClientFinalityUpdateTopicFormat:      func() proto.Message { return &ethpb.LightClientFinalityUpdateAltair{} },
	DataColumnSubnetTopicFormat:               func() proto.Message { return &ethpb.DataColumnSidecar{} },
}

// GossipTopicMappings is a function to return the assigned data type
// versioned by epoch.
func GossipTopicMappings(topic string, epoch primitives.Epoch) proto.Message {
	switch topic {
	case BlockSubnetTopicFormat:
		if epoch >= params.BeaconConfig().FuluForkEpoch {
			return &ethpb.SignedBeaconBlockFulu{}
		}
		if epoch >= params.BeaconConfig().ElectraForkEpoch {
			return &ethpb.SignedBeaconBlockElectra{}
		}
		if epoch >= params.BeaconConfig().DenebForkEpoch {
			return &ethpb.SignedBeaconBlockDeneb{}
		}
		if epoch >= params.BeaconConfig().CapellaForkEpoch {
			return &ethpb.SignedBeaconBlockCapella{}
		}
		if epoch >= params.BeaconConfig().BellatrixForkEpoch {
			return &ethpb.SignedBeaconBlockBellatrix{}
		}
		if epoch >= params.BeaconConfig().AltairForkEpoch {
			return &ethpb.SignedBeaconBlockAltair{}
		}
		return gossipMessage(topic)
	case AttestationSubnetTopicFormat:
		if epoch >= params.BeaconConfig().ElectraForkEpoch {
			return &ethpb.SingleAttestation{}
		}
		return gossipMessage(topic)
	case AttesterSlashingSubnetTopicFormat:
		if epoch >= params.BeaconConfig().ElectraForkEpoch {
			return &ethpb.AttesterSlashingElectra{}
		}
		return gossipMessage(topic)
	case AggregateAndProofSubnetTopicFormat:
		if epoch >= params.BeaconConfig().ElectraForkEpoch {
			return &ethpb.SignedAggregateAttestationAndProofElectra{}
		}
		return gossipMessage(topic)
	case LightClientOptimisticUpdateTopicFormat:
		if epoch >= params.BeaconConfig().DenebForkEpoch {
			return &ethpb.LightClientOptimisticUpdateDeneb{}
		}
		if epoch >= params.BeaconConfig().CapellaForkEpoch {
			return &ethpb.LightClientOptimisticUpdateCapella{}
		}
		return gossipMessage(topic)
	case LightClientFinalityUpdateTopicFormat:
		if epoch >= params.BeaconConfig().ElectraForkEpoch {
			return &ethpb.LightClientFinalityUpdateElectra{}
		}
		if epoch >= params.BeaconConfig().DenebForkEpoch {
			return &ethpb.LightClientFinalityUpdateDeneb{}
		}
		if epoch >= params.BeaconConfig().CapellaForkEpoch {
			return &ethpb.LightClientFinalityUpdateCapella{}
		}
		return gossipMessage(topic)
	default:
		return gossipMessage(topic)
	}
}

func gossipMessage(topic string) proto.Message {
	msgGen, ok := gossipTopicMappings[topic]
	if !ok {
		return nil
	}
	return msgGen()
}

// AllTopics returns all topics stored in our
// gossip mapping.
func AllTopics() []string {
	var topics []string
	for k := range gossipTopicMappings {
		topics = append(topics, k)
	}
	return topics
}

// GossipTypeMapping is the inverse of GossipTopicMappings so that an arbitrary protobuf message
// can be mapped to a protocol ID string.
var GossipTypeMapping = make(map[reflect.Type]string, len(gossipTopicMappings))

func init() {
	for k, v := range gossipTopicMappings {
		GossipTypeMapping[reflect.TypeOf(v())] = k
	}

	// Specially handle Altair objects.
	GossipTypeMapping[reflect.TypeOf(&ethpb.SignedBeaconBlockAltair{})] = BlockSubnetTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.LightClientFinalityUpdateAltair{})] = LightClientFinalityUpdateTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.LightClientOptimisticUpdateAltair{})] = LightClientOptimisticUpdateTopicFormat

	// Specially handle Bellatrix objects.
	GossipTypeMapping[reflect.TypeOf(&ethpb.SignedBeaconBlockBellatrix{})] = BlockSubnetTopicFormat

	// Specially handle Capella objects.
	GossipTypeMapping[reflect.TypeOf(&ethpb.SignedBeaconBlockCapella{})] = BlockSubnetTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.LightClientOptimisticUpdateCapella{})] = LightClientOptimisticUpdateTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.LightClientFinalityUpdateCapella{})] = LightClientFinalityUpdateTopicFormat

	// Specially handle Deneb objects.
	GossipTypeMapping[reflect.TypeOf(&ethpb.SignedBeaconBlockDeneb{})] = BlockSubnetTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.LightClientOptimisticUpdateDeneb{})] = LightClientOptimisticUpdateTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.LightClientFinalityUpdateDeneb{})] = LightClientFinalityUpdateTopicFormat

	// Specially handle Electra objects.
	GossipTypeMapping[reflect.TypeOf(&ethpb.SignedBeaconBlockElectra{})] = BlockSubnetTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.SingleAttestation{})] = AttestationSubnetTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.AttesterSlashingElectra{})] = AttesterSlashingSubnetTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.SignedAggregateAttestationAndProofElectra{})] = AggregateAndProofSubnetTopicFormat
	GossipTypeMapping[reflect.TypeOf(&ethpb.LightClientFinalityUpdateElectra{})] = LightClientFinalityUpdateTopicFormat

	// Specially handle Fulu objects.
	GossipTypeMapping[reflect.TypeOf(&ethpb.SignedBeaconBlockFulu{})] = BlockSubnetTopicFormat
}
