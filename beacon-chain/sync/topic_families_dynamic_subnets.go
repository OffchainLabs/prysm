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

// AttestationTopicFamily
var _ GossipsubTopicFamilyWithDynamicSubnets = (*AttestationTopicFamily)(nil)

type AttestationTopicFamily struct {
	baseGossipsubTopicFamily
}

// NewAttestationTopicFamily creates a new AttestationTopicFamily.
func NewAttestationTopicFamily(s *Service, nse params.NetworkScheduleEntry) *AttestationTopicFamily {
	return &AttestationTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix(),
		},
	}
}

// Validator returns the validator function for attestation subnets.
func (a *AttestationTopicFamily) Validator() wrappedVal {
	return a.syncService.validateCommitteeIndexBeaconAttestation
}

// Handler returns the message handler for attestation subnets.
func (a *AttestationTopicFamily) Handler() subHandler {
	return a.syncService.committeeIndexBeaconAttestationSubscriber
}

// GetFullTopicString builds the full topic string for an attestation subnet.
func (a *AttestationTopicFamily) GetFullTopicString(subnet uint64) string {
	return fmt.Sprintf(p2p.AttestationSubnetTopicFormat, a.nse.ForkDigest, subnet) + a.protocolSuffix
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
	return getTopicsForNode(a.syncService, a, node, a.syncService.cfg.p2p.AttestationSubnets)
}

// TODO
func (a *AttestationTopicFamily) Subscribe() {

}

func (a *AttestationTopicFamily) Unsubscribe() {

}

// SyncCommitteeTopicFamily
var _ GossipsubTopicFamilyWithDynamicSubnets = (*SyncCommitteeTopicFamily)(nil)

type SyncCommitteeTopicFamily struct {
	baseGossipsubTopicFamily
}

// NewSyncCommitteeTopicFamily creates a new SyncCommitteeTopicFamily.
func NewSyncCommitteeTopicFamily(s *Service, nse params.NetworkScheduleEntry) *SyncCommitteeTopicFamily {
	return &SyncCommitteeTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix(),
		},
	}
}

// Validator returns the validator function for sync committee subnets.
func (s *SyncCommitteeTopicFamily) Validator() wrappedVal {
	return s.syncService.validateSyncCommitteeMessage
}

// Handler returns the message handler for sync committee subnets.
func (s *SyncCommitteeTopicFamily) Handler() subHandler {
	return s.syncService.syncCommitteeMessageSubscriber
}

// GetFullTopicString builds the full topic string for a sync committee subnet.
func (s *SyncCommitteeTopicFamily) GetFullTopicString(subnet uint64) string {
	return fmt.Sprintf(p2p.SyncCommitteeSubnetTopicFormat, s.nse.ForkDigest, subnet) + s.protocolSuffix
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
	return getTopicsForNode(s.syncService, s, node, s.syncService.cfg.p2p.SyncSubnets)
}

// TODO
func (s *SyncCommitteeTopicFamily) Subscribe() {

}

func (s *SyncCommitteeTopicFamily) Unsubscribe() {

}

// DataColumnTopicFamily
var _ GossipsubTopicFamilyWithDynamicSubnets = (*DataColumnTopicFamily)(nil)

type DataColumnTopicFamily struct {
	baseGossipsubTopicFamily
}

// NewDataColumnTopicFamily creates a new DataColumnTopicFamily.
func NewDataColumnTopicFamily(s *Service, nse params.NetworkScheduleEntry) *DataColumnTopicFamily {
	return &DataColumnTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix(),
		},
	}
}

// Validator returns the validator function for data column subnets.
func (d *DataColumnTopicFamily) Validator() wrappedVal {
	return d.syncService.validateDataColumn
}

// Handler returns the message handler for data column subnets.
func (d *DataColumnTopicFamily) Handler() subHandler {
	return d.syncService.dataColumnSubscriber
}

// GetFullTopicString builds the full topic string for a data column subnet.
func (d *DataColumnTopicFamily) GetFullTopicString(subnet uint64) string {
	return fmt.Sprintf(p2p.DataColumnSubnetTopicFormat, d.nse.ForkDigest, subnet) + d.protocolSuffix
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
	return getTopicsForNode(d.syncService, d, node, d.syncService.cfg.p2p.DataColumnSubnets)
}

// TODO
func (d *DataColumnTopicFamily) Subscribe() {

}

func (d *DataColumnTopicFamily) Unsubscribe() {

}

type nodeSubnetExtractor func(id enode.ID, n *enode.Node, r *enr.Record) (map[uint64]bool, error)

func getTopicsForNode(
	s *Service,
	tf GossipsubTopicFamilyWithDynamicSubnets,
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
