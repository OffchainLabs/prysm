package sync

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/pkg/errors"
)

// TopicFamily defines an interface for a family of gossip topics,
// such as attestation subnets or sync committee subnets. It encapsulates
// the logic for determining which subnets are needed and how to extract
// them from discovered peers.
type TopicFamily interface {
	// TopicFormat returns the base string format for topics in this family.
	TopicFormat() string
	// GetSubnetsToJoin returns the subnets that our node should be subscribed to at a given slot.
	GetSubnetsToJoin(slot primitives.Slot) map[uint64]bool
	// GetSubnetsForBroadcast returns subnets for which we need to find peers at a given slot.
	GetSubnetsForBroadcast(slot primitives.Slot) map[uint64]bool
	// GetTopicsForNode returns a list of full topics that the given node has for this topic family.
	GetTopicsForNode(node *enode.Node) ([]string, error)
	// ForkDigest returns the fork digest for this topic family.
	ForkDigest() [4]byte
	// GetFullTopicString constructs the full topic string for a given subnet.
	GetFullTopicString(subnet uint64) string
}

type topicFamilyEntry struct {
	activationEpoch primitives.Epoch
	factory         func(s *Service, digest [4]byte) TopicFamily
}

var topicFamilySchedule = []topicFamilyEntry{
	{
		activationEpoch: params.BeaconConfig().GenesisEpoch,
		factory: func(s *Service, digest [4]byte) TopicFamily {
			return NewAttestationTopicFamily(s, digest)
		},
	},
	{
		activationEpoch: params.BeaconConfig().AltairForkEpoch,
		factory: func(s *Service, digest [4]byte) TopicFamily {
			return NewSyncCommitteeTopicFamily(s, digest)
		},
	},
	{
		activationEpoch: params.BeaconConfig().FuluForkEpoch,
		factory: func(s *Service, digest [4]byte) TopicFamily {
			return NewDataColumnTopicFamily(s, digest)
		},
	},
}

func TopicFamiliesForEpoch(epoch primitives.Epoch, s *Service, digest [4]byte) []TopicFamily {
	var activeFamilies []TopicFamily
	for _, entry := range topicFamilySchedule {
		if epoch >= entry.activationEpoch {
			activeFamilies = append(activeFamilies, entry.factory(s, digest))
		}
	}
	return activeFamilies
}

// AttestationTopicFamily implements TopicFamily for attestation subnets.
type AttestationTopicFamily struct {
	syncService *Service
	forkDigest  [4]byte
}

// NewAttestationTopicFamily creates a new AttestationTopicFamily.
func NewAttestationTopicFamily(s *Service, digest [4]byte) *AttestationTopicFamily {
	return &AttestationTopicFamily{syncService: s, forkDigest: digest}
}

// TopicFormat for attestation subnets.
func (a *AttestationTopicFamily) TopicFormat() string {
	return p2p.AttestationSubnetTopicFormat
}

// GetSubnetsToJoin returns persistent and aggregator subnets.
func (a *AttestationTopicFamily) GetSubnetsToJoin(slot primitives.Slot) map[uint64]bool {
	return a.syncService.persistentAndAggregatorSubnetIndices(slot)
}

// GetSubnetsForBroadcast returns subnets needed for attestation duties.
func (a *AttestationTopicFamily) GetSubnetsForBroadcast(slot primitives.Slot) map[uint64]bool {
	return attesterSubnetIndices(slot)
}

// GetTopicsForNode returns all topics for the given node that are relevant to this topic family.
func (a *AttestationTopicFamily) GetTopicsForNode(node *enode.Node) ([]string, error) {
	return getTopicsForNodeHelper(a.syncService, a, node, a.syncService.cfg.p2p.AttestationSubnets)
}

// ForkDigest returns the fork digest.
func (a *AttestationTopicFamily) ForkDigest() [4]byte {
	return a.forkDigest
}

// GetFullTopicString builds the full topic string for an attestation subnet.
func (a *AttestationTopicFamily) GetFullTopicString(subnet uint64) string {
	return fmt.Sprintf(a.TopicFormat(), a.forkDigest, subnet) + a.syncService.cfg.p2p.Encoding().ProtocolSuffix()
}

// SyncCommitteeTopicFamily implements TopicFamily for sync committee subnets.
type SyncCommitteeTopicFamily struct {
	syncService *Service
	forkDigest  [4]byte
}

// NewSyncCommitteeTopicFamily creates a new SyncCommitteeTopicFamily.
func NewSyncCommitteeTopicFamily(s *Service, digest [4]byte) *SyncCommitteeTopicFamily {
	return &SyncCommitteeTopicFamily{syncService: s, forkDigest: digest}
}

// TopicFormat for sync committee subnets.
func (s *SyncCommitteeTopicFamily) TopicFormat() string {
	return p2p.SyncCommitteeSubnetTopicFormat
}

// GetSubnetsToJoin returns active sync committee subnets.
func (s *SyncCommitteeTopicFamily) GetSubnetsToJoin(slot primitives.Slot) map[uint64]bool {
	return s.syncService.activeSyncSubnetIndices(slot)
}

// GetSubnetsForBroadcast returns nil as there are no separate peer requirements.
func (s *SyncCommitteeTopicFamily) GetSubnetsForBroadcast(slot primitives.Slot) map[uint64]bool {
	return nil
}

// GetTopicsForNode returns all topics for the given node that are relevant to this topic family.
func (s *SyncCommitteeTopicFamily) GetTopicsForNode(node *enode.Node) ([]string, error) {
	return getTopicsForNodeHelper(s.syncService, s, node, s.syncService.cfg.p2p.SyncSubnets)
}

// ForkDigest returns the fork digest.
func (s *SyncCommitteeTopicFamily) ForkDigest() [4]byte {
	return s.forkDigest
}

// GetFullTopicString builds the full topic string for a sync committee subnet.
func (s *SyncCommitteeTopicFamily) GetFullTopicString(subnet uint64) string {
	return fmt.Sprintf(s.TopicFormat(), s.forkDigest, subnet) + s.syncService.cfg.p2p.Encoding().ProtocolSuffix()
}

// DataColumnTopicFamily implements TopicFamily for data column subnets.
type DataColumnTopicFamily struct {
	syncService *Service
	forkDigest  [4]byte
}

// NewDataColumnTopicFamily creates a new DataColumnTopicFamily.
func NewDataColumnTopicFamily(s *Service, digest [4]byte) *DataColumnTopicFamily {
	return &DataColumnTopicFamily{syncService: s, forkDigest: digest}
}

// TopicFormat for data column subnets.
func (d *DataColumnTopicFamily) TopicFormat() string {
	return p2p.DataColumnSubnetTopicFormat
}

// GetSubnetsToJoin returns data column subnets.
func (d *DataColumnTopicFamily) GetSubnetsToJoin(slot primitives.Slot) map[uint64]bool {
	return d.syncService.dataColumnSubnetIndices(slot)
}

// GetSubnetsForBroadcast returns all data column subnets.
func (d *DataColumnTopicFamily) GetSubnetsForBroadcast(slot primitives.Slot) map[uint64]bool {
	return d.syncService.allDataColumnSubnets(slot)
}

// GetTopicsForNode returns all topics for the given node that are relevant to this topic family.
func (d *DataColumnTopicFamily) GetTopicsForNode(node *enode.Node) ([]string, error) {
	return getTopicsForNodeHelper(d.syncService, d, node, d.syncService.cfg.p2p.DataColumnSubnets)
}

// ForkDigest returns the fork digest.
func (d *DataColumnTopicFamily) ForkDigest() [4]byte {
	return d.forkDigest
}

// GetFullTopicString builds the full topic string for a data column subnet.
func (d *DataColumnTopicFamily) GetFullTopicString(subnet uint64) string {
	return fmt.Sprintf(d.TopicFormat(), d.forkDigest, subnet) + d.syncService.cfg.p2p.Encoding().ProtocolSuffix()
}

type nodeSubnetExtractor func(id enode.ID, n *enode.Node, r *enr.Record) (map[uint64]bool, error)

func getTopicsForNodeHelper(
	s *Service,
	tf TopicFamily,
	node *enode.Node,
	extractor nodeSubnetExtractor,
) ([]string, error) {
	if node == nil {
		return nil, errors.New("enode is nil")
	}
	currentSlot := s.cfg.clock.CurrentSlot()
	neededSubnets := computeAllNeededSubnets(
		currentSlot,
		tf.GetSubnetsToJoin,
		tf.GetSubnetsForBroadcast,
	)

	nodeSubnets, err := extractor(node.ID(), node, node.Record())
	if err != nil {
		return nil, err
	}

	var topics []string
	for subnet := range neededSubnets {
		if nodeSubnets[subnet] {
			topics = append(topics, tf.GetFullTopicString(subnet))
		}
	}
	return topics, nil
}
