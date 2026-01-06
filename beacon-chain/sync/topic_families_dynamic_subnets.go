package sync

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/pkg/errors"
)

// AttestationTopicFamily
var _ DynamicShardedTopicFamily = (*AttestationTopicFamily)(nil)

var (
	attestationMinMeshPeers     = 8
	attestationMinFanoutPeers   = 6
	syncCommitteeMinMeshPeers   = 8
	syncCommitteeMinFanoutPeers = 6
	dataColumnMinMeshPeers      = 6
	dataColumnMinFanoutPeers    = 2
)

type AttestationTopicFamily struct {
	*baseTopicFamily
}

// NewAttestationTopicFamily creates a new AttestationTopicFamily.
func NewAttestationTopicFamily(s *Service, nse params.NetworkScheduleEntry) *AttestationTopicFamily {
	a := &AttestationTopicFamily{}
	base := newBaseTopicFamily(s, nse, s.validateCommitteeIndexBeaconAttestation, s.committeeIndexBeaconAttestationSubscriber, a)
	a.baseTopicFamily = base
	return a
}

func (a *AttestationTopicFamily) Name() string {
	return "AttestationTopicFamily"
}

// SubscribeForSlot subscribes to the topics for the given slot.
func (a *AttestationTopicFamily) SubscribeForSlot(slot primitives.Slot) {
	a.subscribeToTopics(a.TopicsToSubscribeForSlot(slot))
}

// UnsubscribeForSlot unsubscribes from topics we no longer need for the slot.
func (a *AttestationTopicFamily) UnsubscribeForSlot(slot primitives.Slot) {
	a.pruneTopicsExcept(a.TopicsToSubscribeForSlot(slot))
}

// TopicsToSubscribeFor returns the topics to subscribe to for a given slot.
func (a *AttestationTopicFamily) TopicsToSubscribeForSlot(slot primitives.Slot) []string {
	return topicsFromSubnets(a.getSubnetsToJoin(slot), a)
}

// getFullTopicString builds the full topic string for an attestation subnet.
func (a *AttestationTopicFamily) getFullTopicString(subnet uint64) string {
	return p2p.AttestationSubnetTopic(a.nse.ForkDigest, subnet)
}

// getSubnetsToJoin returns persistent and aggregator subnets.
func (a *AttestationTopicFamily) getSubnetsToJoin(slot primitives.Slot) map[uint64]bool {
	return a.syncService.persistentAndAggregatorSubnetIndices(slot)
}

// getSubnetsForBroadcast returns subnets needed for attestation duties.
func (a *AttestationTopicFamily) getSubnetsForBroadcast(slot primitives.Slot) map[uint64]bool {
	return attesterSubnetIndices(slot)
}

// ExtractTopicsForNode returns all topics for the given node that are relevant to this topic family.
func (a *AttestationTopicFamily) ExtractTopicsForNode(node *enode.Node) ([]string, error) {
	return getTopicsForNode(a.syncService, a, node, p2p.AttestationSubnets)
}

// TopicsWithMinPeerCount returns all topics (mesh and fanout) with their respective min peer counts.
func (a *AttestationTopicFamily) TopicsWithMinPeerCount(slot primitives.Slot) map[string]int {
	return topicsWithMinPeerCount(a, slot, attestationMinMeshPeers, attestationMinFanoutPeers)
}

// SyncCommitteeTopicFamily
var _ DynamicShardedTopicFamily = (*SyncCommitteeTopicFamily)(nil)

type SyncCommitteeTopicFamily struct {
	*baseTopicFamily
}

// NewSyncCommitteeTopicFamily creates a new SyncCommitteeTopicFamily.
func NewSyncCommitteeTopicFamily(s *Service, nse params.NetworkScheduleEntry) *SyncCommitteeTopicFamily {
	sc := &SyncCommitteeTopicFamily{}
	base := newBaseTopicFamily(s, nse, s.validateSyncCommitteeMessage, s.syncCommitteeMessageSubscriber, sc)
	sc.baseTopicFamily = base
	return sc
}

func (s *SyncCommitteeTopicFamily) Name() string {
	return "SyncCommitteeTopicFamily"
}

// SubscribeFor subscribes to the topics for the given slot.
func (s *SyncCommitteeTopicFamily) SubscribeForSlot(slot primitives.Slot) {
	s.subscribeToTopics(s.TopicsToSubscribeForSlot(slot))
}

// UnsubscribeFor unsubscribes from topics we no longer need for the slot.
func (s *SyncCommitteeTopicFamily) UnsubscribeForSlot(slot primitives.Slot) {
	s.pruneTopicsExcept(s.TopicsToSubscribeForSlot(slot))
}

// TopicsToSubscribeFor returns the topics to subscribe to for a given slot.
func (s *SyncCommitteeTopicFamily) TopicsToSubscribeForSlot(slot primitives.Slot) []string {
	return topicsFromSubnets(s.getSubnetsToJoin(slot), s)
}

// getFullTopicString builds the full topic string for a sync committee subnet.
func (s *SyncCommitteeTopicFamily) getFullTopicString(subnet uint64) string {
	return p2p.SyncCommitteeSubnetTopic(s.nse.ForkDigest, subnet)
}

// getSubnetsToJoin returns active sync committee subnets.
func (s *SyncCommitteeTopicFamily) getSubnetsToJoin(slot primitives.Slot) map[uint64]bool {
	return s.syncService.activeSyncSubnetIndices(slot)
}

// getSubnetsForBroadcast returns nil as there are no separate peer requirements.
func (s *SyncCommitteeTopicFamily) getSubnetsForBroadcast(slot primitives.Slot) map[uint64]bool {
	return nil
}

// ExtractTopicsForNode returns all topics for the given node that are relevant to this topic family.
func (s *SyncCommitteeTopicFamily) ExtractTopicsForNode(node *enode.Node) ([]string, error) {
	return getTopicsForNode(s.syncService, s, node, p2p.SyncSubnets)
}

// TopicsWithMinPeerCount returns all topics (mesh and fanout) with their respective min peer counts.
func (s *SyncCommitteeTopicFamily) TopicsWithMinPeerCount(slot primitives.Slot) map[string]int {
	return topicsWithMinPeerCount(s, slot, syncCommitteeMinMeshPeers, syncCommitteeMinFanoutPeers)
}

// DataColumnTopicFamily
var _ DynamicShardedTopicFamily = (*DataColumnTopicFamily)(nil)

type DataColumnTopicFamily struct {
	*baseTopicFamily
}

// NewDataColumnTopicFamily creates a new DataColumnTopicFamily.
func NewDataColumnTopicFamily(s *Service, nse params.NetworkScheduleEntry) *DataColumnTopicFamily {
	d := &DataColumnTopicFamily{}
	base := newBaseTopicFamily(s, nse, s.validateDataColumn, s.dataColumnSubscriber, d)
	d.baseTopicFamily = base
	return d
}

func (d *DataColumnTopicFamily) Name() string {
	return "DataColumnTopicFamily"
}

// SubscribeFor subscribes to the topics for the given slot.
func (d *DataColumnTopicFamily) SubscribeForSlot(slot primitives.Slot) {
	d.subscribeToTopics(d.TopicsToSubscribeForSlot(slot))
}

// UnsubscribeForSlot unsubscribes from topics we no longer need for the slot.
func (d *DataColumnTopicFamily) UnsubscribeForSlot(slot primitives.Slot) {
	d.pruneTopicsExcept(d.TopicsToSubscribeForSlot(slot))
}

// TopicsToSubscribeFor returns the topics to subscribe to for a given slot.
func (d *DataColumnTopicFamily) TopicsToSubscribeForSlot(slot primitives.Slot) []string {
	return topicsFromSubnets(d.getSubnetsToJoin(slot), d)
}

// getFullTopicString builds the full topic string for a data column subnet.
func (d *DataColumnTopicFamily) getFullTopicString(subnet uint64) string {
	return p2p.DataColumnSubnetTopic(d.nse.ForkDigest, subnet)
}

// getSubnetsToJoin returns data column subnets.
func (d *DataColumnTopicFamily) getSubnetsToJoin(slot primitives.Slot) map[uint64]bool {
	return d.syncService.dataColumnSubnetIndices(slot)
}

// getSubnetsForBroadcast returns all data column subnets.
func (d *DataColumnTopicFamily) getSubnetsForBroadcast(slot primitives.Slot) map[uint64]bool {
	return d.syncService.allDataColumnSubnets(slot)
}

// ExtractTopicsForNode returns all topics for the given node that are relevant to this topic family.
func (d *DataColumnTopicFamily) ExtractTopicsForNode(node *enode.Node) ([]string, error) {
	return getTopicsForNode(d.syncService, d, node, p2p.DataColumnSubnets)
}

// TopicsWithMinPeerCount returns all topics (mesh and fanout) with their respective min peer counts.
func (d *DataColumnTopicFamily) TopicsWithMinPeerCount(slot primitives.Slot) map[string]int {
	return topicsWithMinPeerCount(d, slot, dataColumnMinMeshPeers, dataColumnMinFanoutPeers)
}

type nodeSubnetExtractor func(id enode.ID, n *enode.Node, r *enr.Record) (map[uint64]bool, error)

type dynamicSubnetFamily interface {
	getSubnetsToJoin(primitives.Slot) map[uint64]bool
	getSubnetsForBroadcast(primitives.Slot) map[uint64]bool
	getFullTopicString(subnet uint64) string
}

func getTopicsForNode(
	s *Service,
	tf dynamicSubnetFamily,
	node *enode.Node,
	extractor nodeSubnetExtractor,
) ([]string, error) {
	if node == nil {
		return nil, errors.New("enode is nil")
	}
	currentSlot := s.cfg.clock.CurrentSlot()
	neededSubnets := computeNeededSubnets(tf, currentSlot)

	nodeSubnets, err := extractor(node.ID(), node, node.Record())
	if err != nil {
		return nil, err
	}

	var topics []string
	for subnet := range neededSubnets {
		if nodeSubnets[subnet] {
			topics = append(topics, tf.getFullTopicString(subnet))
		}
	}
	return topics, nil
}

func computeNeededSubnets(tf dynamicSubnetFamily, slot primitives.Slot) map[uint64]bool {
	subnetsToJoin := tf.getSubnetsToJoin(slot)
	subnetsRequiringPeers := tf.getSubnetsForBroadcast(slot)

	neededSubnets := make(map[uint64]bool, len(subnetsToJoin)+len(subnetsRequiringPeers))
	for subnet := range subnetsToJoin {
		neededSubnets[subnet] = true
	}
	for subnet := range subnetsRequiringPeers {
		neededSubnets[subnet] = true
	}
	return neededSubnets
}

func topicsFromSubnets(subnets map[uint64]bool, tf dynamicSubnetFamily) []string {
	topics := make([]string, 0, len(subnets))
	for s := range subnets {
		topics = append(topics, tf.getFullTopicString(s))
	}
	return topics
}

// topicsWithMinPeerCount returns all topics (mesh and fanout) with their respective min peer counts.
// If a subnet appears in both mesh and fanout, the mesh peer count is used.
func topicsWithMinPeerCount(tf dynamicSubnetFamily, slot primitives.Slot, minMeshPeers int, minFanoutPeers int) map[string]int {
	meshSubnets := tf.getSubnetsToJoin(slot)
	fanoutSubnets := tf.getSubnetsForBroadcast(slot)

	result := make(map[string]int, len(meshSubnets)+len(fanoutSubnets))

	// Add mesh topics with mesh min peer count
	for subnet := range meshSubnets {
		topic := tf.getFullTopicString(subnet)
		result[topic] = minMeshPeers
	}

	// Add fanout topics with fanout min peer count (only if not already in mesh)
	for subnet := range fanoutSubnets {
		topic := tf.getFullTopicString(subnet)
		if _, exists := result[topic]; !exists {
			result[topic] = minFanoutPeers
		}
	}

	return result
}
