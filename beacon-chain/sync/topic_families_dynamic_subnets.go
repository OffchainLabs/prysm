package sync

import (
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// AttestationTopicFamily
var _ GossipsubTopicFamilyWithDynamicSubnets = (*AttestationTopicFamily)(nil)

type baseGossipsubTopicFamilyWithDynamicSubnets struct {
	baseGossipsubTopicFamily

	mu           sync.Mutex
	tracker      *subnetTracker
	unsubscribed bool
}

func (b *baseGossipsubTopicFamilyWithDynamicSubnets) Subscribe(tf GossipsubTopicFamilyWithDynamicSubnets) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.unsubscribed {
		log.WithFields(logrus.Fields{
			"topicFamily": fmt.Sprintf("%T", tf),
			"digest":      b.nse.ForkDigest,
			"epoch":       b.nse.Epoch,
		}).Error("Cannot subscribe after unsubscribing")
		return
	}
	b.tracker = b.syncService.subscribeToDynamicSubnetFamily(tf)
}

func (b *baseGossipsubTopicFamilyWithDynamicSubnets) Unsubscribe() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.unsubscribed = true
	b.syncService.pruneNotWanted(b.tracker, nil) // unsubscribe from all subnets
}

type AttestationTopicFamily struct {
	baseGossipsubTopicFamilyWithDynamicSubnets
}

// NewAttestationTopicFamily creates a new AttestationTopicFamily.
func NewAttestationTopicFamily(s *Service, nse params.NetworkScheduleEntry) *AttestationTopicFamily {
	attestationTopicFamily := &AttestationTopicFamily{
		baseGossipsubTopicFamilyWithDynamicSubnets: baseGossipsubTopicFamilyWithDynamicSubnets{
			baseGossipsubTopicFamily: baseGossipsubTopicFamily{
				syncService:    s,
				nse:            nse,
				protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix(),
			},
		},
	}
	return attestationTopicFamily
}

func (a *AttestationTopicFamily) Name() string {
	return "AttestationTopicFamily"
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
	return getTopicsForNode(a.syncService, a, node, p2p.AttestationSubnets)
}

func (a *AttestationTopicFamily) Subscribe() {
	a.baseGossipsubTopicFamilyWithDynamicSubnets.Subscribe(a)
}

func (a *AttestationTopicFamily) Unsubscribe() {
	a.baseGossipsubTopicFamilyWithDynamicSubnets.Unsubscribe()
}

// SyncCommitteeTopicFamily
var _ GossipsubTopicFamilyWithDynamicSubnets = (*SyncCommitteeTopicFamily)(nil)

type SyncCommitteeTopicFamily struct {
	baseGossipsubTopicFamilyWithDynamicSubnets
}

// NewSyncCommitteeTopicFamily creates a new SyncCommitteeTopicFamily.
func NewSyncCommitteeTopicFamily(s *Service, nse params.NetworkScheduleEntry) *SyncCommitteeTopicFamily {
	return &SyncCommitteeTopicFamily{
		baseGossipsubTopicFamilyWithDynamicSubnets: baseGossipsubTopicFamilyWithDynamicSubnets{
			baseGossipsubTopicFamily: baseGossipsubTopicFamily{
				syncService:    s,
				nse:            nse,
				protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix(),
			},
		},
	}
}

func (s *SyncCommitteeTopicFamily) Name() string {
	return "SyncCommitteeTopicFamily"
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
	return getTopicsForNode(s.syncService, s, node, p2p.SyncSubnets)
}

func (s *SyncCommitteeTopicFamily) Subscribe() {
	s.baseGossipsubTopicFamilyWithDynamicSubnets.Subscribe(s)
}

func (s *SyncCommitteeTopicFamily) Unsubscribe() {
	s.baseGossipsubTopicFamilyWithDynamicSubnets.Unsubscribe()
}

// DataColumnTopicFamily
var _ GossipsubTopicFamilyWithDynamicSubnets = (*DataColumnTopicFamily)(nil)

type DataColumnTopicFamily struct {
	baseGossipsubTopicFamilyWithDynamicSubnets
}

// NewDataColumnTopicFamily creates a new DataColumnTopicFamily.
func NewDataColumnTopicFamily(s *Service, nse params.NetworkScheduleEntry) *DataColumnTopicFamily {
	return &DataColumnTopicFamily{
		baseGossipsubTopicFamilyWithDynamicSubnets: baseGossipsubTopicFamilyWithDynamicSubnets{
			baseGossipsubTopicFamily: baseGossipsubTopicFamily{
				syncService:    s,
				nse:            nse,
				protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix(),
			},
		},
	}
}

func (d *DataColumnTopicFamily) Name() string {
	return "DataColumnTopicFamily"
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
	return getTopicsForNode(d.syncService, d, node, p2p.DataColumnSubnets)
}

func (d *DataColumnTopicFamily) Subscribe() {
	d.baseGossipsubTopicFamilyWithDynamicSubnets.Subscribe(d)
}

func (d *DataColumnTopicFamily) Unsubscribe() {
	d.baseGossipsubTopicFamilyWithDynamicSubnets.Unsubscribe()
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
		tf,
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
