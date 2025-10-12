package p2p

import (
	"encoding/hex"
	"slices"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

const (
	// GossipProtocolAndDigest represents the protocol and fork digest prefix in a gossip topic.
	GossipProtocolAndDigest = "/eth2/%x/"

	// Message Types
	//
	// GossipAttestationMessage is the name for the attestation message type. It is
	// specially extracted so as to determine the correct message type from an attestation
	// subnet.
	GossipAttestationMessage = "beacon_attestation"
	// GossipSyncCommitteeMessage is the name for the sync committee message type. It is
	// specially extracted so as to determine the correct message type from a sync committee
	// subnet.
	GossipSyncCommitteeMessage = "sync_committee"
	// GossipBlockMessage is the name for the block message type.
	GossipBlockMessage = "beacon_block"
	// GossipExitMessage is the name for the voluntary exit message type.
	GossipExitMessage = "voluntary_exit"
	// GossipProposerSlashingMessage is the name for the proposer slashing message type.
	GossipProposerSlashingMessage = "proposer_slashing"
	// GossipAttesterSlashingMessage is the name for the attester slashing message type.
	GossipAttesterSlashingMessage = "attester_slashing"
	// GossipAggregateAndProofMessage is the name for the attestation aggregate and proof message type.
	GossipAggregateAndProofMessage = "beacon_aggregate_and_proof"
	// GossipContributionAndProofMessage is the name for the sync contribution and proof message type.
	GossipContributionAndProofMessage = "sync_committee_contribution_and_proof"
	// GossipBlsToExecutionChangeMessage is the name for the bls to execution change message type.
	GossipBlsToExecutionChangeMessage = "bls_to_execution_change"
	// GossipBlobSidecarMessage is the name for the blob sidecar message type.
	GossipBlobSidecarMessage = "blob_sidecar"
	// GossipLightClientFinalityUpdateMessage is the name for the light client finality update message type.
	GossipLightClientFinalityUpdateMessage = "light_client_finality_update"
	// GossipLightClientOptimisticUpdateMessage is the name for the light client optimistic update message type.
	GossipLightClientOptimisticUpdateMessage = "light_client_optimistic_update"
	// GossipDataColumnSidecarMessage is the name for the data column sidecar message type.
	GossipDataColumnSidecarMessage = "data_column_sidecar"

	// Topic Formats
	//
	// AttestationSubnetTopicFormat is the topic format for the attestation subnet.
	AttestationSubnetTopicFormat = GossipProtocolAndDigest + GossipAttestationMessage + "_%d"
	// SyncCommitteeSubnetTopicFormat is the topic format for the sync committee subnet.
	SyncCommitteeSubnetTopicFormat = GossipProtocolAndDigest + GossipSyncCommitteeMessage + "_%d"
	// BlockSubnetTopicFormat is the topic format for the block subnet.
	BlockSubnetTopicFormat = GossipProtocolAndDigest + GossipBlockMessage
	// ExitSubnetTopicFormat is the topic format for the voluntary exit subnet.
	ExitSubnetTopicFormat = GossipProtocolAndDigest + GossipExitMessage
	// ProposerSlashingSubnetTopicFormat is the topic format for the proposer slashing subnet.
	ProposerSlashingSubnetTopicFormat = GossipProtocolAndDigest + GossipProposerSlashingMessage
	// AttesterSlashingSubnetTopicFormat is the topic format for the attester slashing subnet.
	AttesterSlashingSubnetTopicFormat = GossipProtocolAndDigest + GossipAttesterSlashingMessage
	// AggregateAndProofSubnetTopicFormat is the topic format for the aggregate and proof subnet.
	AggregateAndProofSubnetTopicFormat = GossipProtocolAndDigest + GossipAggregateAndProofMessage
	// SyncContributionAndProofSubnetTopicFormat is the topic format for the sync aggregate and proof subnet.
	SyncContributionAndProofSubnetTopicFormat = GossipProtocolAndDigest + GossipContributionAndProofMessage
	// BlsToExecutionChangeSubnetTopicFormat is the topic format for the bls to execution change subnet.
	BlsToExecutionChangeSubnetTopicFormat = GossipProtocolAndDigest + GossipBlsToExecutionChangeMessage
	// BlobSubnetTopicFormat is the topic format for the blob subnet.
	BlobSubnetTopicFormat = GossipProtocolAndDigest + GossipBlobSidecarMessage + "_%d"
	// LightClientFinalityUpdateTopicFormat is the topic format for the light client finality update subnet.
	LightClientFinalityUpdateTopicFormat = GossipProtocolAndDigest + GossipLightClientFinalityUpdateMessage
	// LightClientOptimisticUpdateTopicFormat is the topic format for the light client optimistic update subnet.
	LightClientOptimisticUpdateTopicFormat = GossipProtocolAndDigest + GossipLightClientOptimisticUpdateMessage
	// DataColumnSubnetTopicFormat is the topic format for the data column subnet.
	DataColumnSubnetTopicFormat = GossipProtocolAndDigest + GossipDataColumnSidecarMessage + "_%d"
)

// topic is a struct representing a single gossipsub topic.
// It can also be used to represent a set of subnet topics: see appendSubnetsBelow().
// topic is intended to be used as an immutable value - it is hashable so it can be used as a map key
// and it uses strings in order to leverage golangs string interning for memory efficiency.
type topic struct {
	full    string
	digest  string
	message string
	start   primitives.Epoch
	end     primitives.Epoch
	suffix  string
	subnet  uint64
}

func (t topic) String() string {
	return t.full
}

// sszEnc is used to get the protocol suffix for topics. This value has been effectively hardcoded
// since phase0.
var sszEnc = &encoder.SszNetworkEncoder{}

// newTopic constructs a topic value for an ordinary topic structure (without subnets).
func newTopic(start, end primitives.Epoch, digest [4]byte, message string) topic {
	suffix := sszEnc.ProtocolSuffix()
	t := topic{digest: hex.EncodeToString(digest[:]), message: message, start: start, end: end, suffix: suffix}
	t.full = "/" + "eth2" + "/" + t.digest + "/" + t.message + t.suffix
	return t
}

// newSubnetTopic constructs a topic value for a topic with a subnet structure.
func newSubnetTopic(start, end primitives.Epoch, digest [4]byte, message string, subnet uint64) topic {
	t := newTopic(start, end, digest, message)
	t.subnet = subnet
	t.full = "/" + "eth2" + "/" + t.digest + "/" + t.message + "_" + strconv.Itoa(int(t.subnet)) + t.suffix
	return t
}

// allTopicStrings returns the full topic string for all topics
// that could be derived from the current fork schedule.
func (s *Service) allTopicStrings() []string {
	topics := s.allTopics()
	topicStrs := make([]string, 0, len(topics))
	for _, t := range topics {
		topicStrs = append(topicStrs, t.String())
	}
	return topicStrs
}

// appendSubnetsBelow uses the value of top.subnet as the subnet count
// and creates a topic value for each subnet less than the subnet count, appending them all
// to appendTo.
func appendSubnetsBelow(top topic, digest [4]byte, appendTo []topic) []topic {
	for i := range top.subnet {
		appendTo = append(appendTo, newSubnetTopic(top.start, top.end, digest, top.message, i))
	}
	return appendTo
}

// allTopics returns all topics that could be derived from the current fork schedule.
func (s *Service) allTopics() []topic {
	cfg := params.BeaconConfig()
	// bellatrix: no special topics; electra: blobs topics handled all together
	genesis, altair, capella := cfg.GenesisEpoch, cfg.AltairForkEpoch, cfg.CapellaForkEpoch
	deneb, fulu, future := cfg.DenebForkEpoch, cfg.FuluForkEpoch, cfg.FarFutureEpoch
	// Templates are starter topics - they have a placeholder digest and the subnet is set to the maximum value
	// for the subnet (see how this is used in allSubnetsBelow). These are not directly returned by the method,
	// they are copied and modified for each digest where they apply based on the start and end epochs.
	empty := [4]byte{0, 0, 0, 0} // empty digest for templates, replaced by real digests in per-fork copies.
	templates := []topic{
		newTopic(genesis, future, empty, GossipBlockMessage),
		newTopic(genesis, future, empty, GossipAggregateAndProofMessage),
		newTopic(genesis, future, empty, GossipExitMessage),
		newTopic(genesis, future, empty, GossipProposerSlashingMessage),
		newTopic(genesis, future, empty, GossipAttesterSlashingMessage),
		newSubnetTopic(genesis, future, empty, GossipAttestationMessage, cfg.AttestationSubnetCount),
		newSubnetTopic(altair, future, empty, GossipSyncCommitteeMessage, cfg.SyncCommitteeSubnetCount),
		newTopic(altair, future, empty, GossipContributionAndProofMessage),
		newTopic(altair, future, empty, GossipLightClientOptimisticUpdateMessage),
		newTopic(altair, future, empty, GossipLightClientFinalityUpdateMessage),
		newTopic(capella, future, empty, GossipBlsToExecutionChangeMessage),
	}
	last := params.GetNetworkScheduleEntry(genesis)
	schedule := []params.NetworkScheduleEntry{last}
	for next := params.NextNetworkScheduleEntry(last.Epoch); next.ForkDigest != last.ForkDigest; next = params.NextNetworkScheduleEntry(next.Epoch) {
		schedule = append(schedule, next)
		last = next
	}
	slices.Reverse(schedule) // reverse the fork schedule because it simplifies dealing with BPOs
	fullTopics := make([]topic, 0, len(templates))
	for _, top := range templates {
		for _, entry := range schedule {
			if top.start <= entry.Epoch && entry.Epoch < top.end {
				if top.subnet > 0 { // subnet topics in the list above should set this value to the max subnet count: see allSubnetsBelow
					fullTopics = appendSubnetsBelow(top, entry.ForkDigest, fullTopics)
				} else {
					fullTopics = append(fullTopics, newTopic(top.start, top.end, entry.ForkDigest, top.message))
				}
			}
		}
	}
	end := future
	// We're iterating from high to low per the slices.Reverse above.
	// So we'll update end = n.Epoch as we go down, and use that as the end for the next entry.
	// This loop either adds blob or data column sidecar topics depending on the fork.
	for _, entry := range schedule {
		if entry.Epoch < deneb {
			break
			// note: there is a special case where deneb is the genesis fork, in which case
			// we'll generate blob sidecar topics for the earlier schedule, but
			// this only happens in devnets where it doesn't really matter.
		}
		message := GossipDataColumnSidecarMessage
		subnets := cfg.DataColumnSidecarSubnetCount
		if entry.Epoch < fulu {
			message = GossipBlobSidecarMessage
			subnets = uint64(cfg.MaxBlobsPerBlockAtEpoch(entry.Epoch))
		}
		// Set subnet to max value, allSubnetsBelow will iterate every index up to that value.
		top := newSubnetTopic(entry.Epoch, end, entry.ForkDigest, message, subnets)
		fullTopics = appendSubnetsBelow(top, entry.ForkDigest, fullTopics)
		end = entry.Epoch // These topics / subnet structures are mutually exclusive, so set each end to the next highest entry.
	}
	return fullTopics
}
