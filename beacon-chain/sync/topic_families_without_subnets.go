package sync

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
)

// Blocks
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*BlockTopicFamily)(nil)

type BlockTopicFamily struct {
	*baseGossipsubTopicFamily
}

func NewBlockTopicFamily(s *Service, nse params.NetworkScheduleEntry) *BlockTopicFamily {
	b := &BlockTopicFamily{}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateBeaconBlockPubSub, s.beaconBlockSubscriber, b)
	b.baseGossipsubTopicFamily = base
	return b
}

func (b *BlockTopicFamily) Name() string {
	return "BlockTopicFamily"
}

// Subscribe subscribes to the topic.
func (b *BlockTopicFamily) Subscribe() {
	b.subscribeToTopics([]string{b.getFullTopicString()})
}

// UnsubscribeAll unsubscribes from all topics in the family.
func (b *BlockTopicFamily) UnsubscribeAll() {
	b.unsubscribeAll()
}

func (b *BlockTopicFamily) getFullTopicString() string {
	return p2p.BlockSubnetTopic(b.nse.ForkDigest)
}

// Aggregate and Proof
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*AggregateAndProofTopicFamily)(nil)

type AggregateAndProofTopicFamily struct {
	*baseGossipsubTopicFamily
}

func NewAggregateAndProofTopicFamily(s *Service, nse params.NetworkScheduleEntry) *AggregateAndProofTopicFamily {
	a := &AggregateAndProofTopicFamily{}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateAggregateAndProof, s.beaconAggregateProofSubscriber, a)
	a.baseGossipsubTopicFamily = base
	return a
}

func (a *AggregateAndProofTopicFamily) Name() string {
	return "AggregateAndProofTopicFamily"
}

// Subscribe subscribes to the topic.
func (a *AggregateAndProofTopicFamily) Subscribe() {
	a.subscribeToTopics([]string{a.getFullTopicString()})
}

// UnsubscribeAll unsubscribes from all topics in the family.
func (a *AggregateAndProofTopicFamily) UnsubscribeAll() {
	a.unsubscribeAll()
}

func (a *AggregateAndProofTopicFamily) getFullTopicString() string {
	return p2p.AggregateAndProofSubnetTopic(a.nse.ForkDigest)
}

// Voluntary Exit
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*VoluntaryExitTopicFamily)(nil)

type VoluntaryExitTopicFamily struct {
	*baseGossipsubTopicFamily
}

func NewVoluntaryExitTopicFamily(s *Service, nse params.NetworkScheduleEntry) *VoluntaryExitTopicFamily {
	v := &VoluntaryExitTopicFamily{}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateVoluntaryExit, s.voluntaryExitSubscriber, v)
	v.baseGossipsubTopicFamily = base
	return v
}

func (v *VoluntaryExitTopicFamily) Name() string {
	return "VoluntaryExitTopicFamily"
}

// Subscribe subscribes to the topic. Slot is ignored for this topic family.
func (v *VoluntaryExitTopicFamily) Subscribe() {
	v.subscribeToTopics([]string{v.getFullTopicString()})
}

// UnsubscribeAll unsubscribes from all topics in the family.
func (v *VoluntaryExitTopicFamily) UnsubscribeAll() {
	v.unsubscribeAll()
}

func (v *VoluntaryExitTopicFamily) getFullTopicString() string {
	return p2p.VoluntaryExitSubnetTopic(v.nse.ForkDigest)
}

// Proposer Slashing
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*ProposerSlashingTopicFamily)(nil)

type ProposerSlashingTopicFamily struct {
	*baseGossipsubTopicFamily
}

func NewProposerSlashingTopicFamily(s *Service, nse params.NetworkScheduleEntry) *ProposerSlashingTopicFamily {
	p := &ProposerSlashingTopicFamily{}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateProposerSlashing, s.proposerSlashingSubscriber, p)
	p.baseGossipsubTopicFamily = base
	return p
}

func (p *ProposerSlashingTopicFamily) Name() string {
	return "ProposerSlashingTopicFamily"
}

// Subscribe subscribes to the topic. Slot is ignored for this topic family.
func (p *ProposerSlashingTopicFamily) Subscribe() {
	p.subscribeToTopics([]string{p.getFullTopicString()})
}

// UnsubscribeAll unsubscribes from all topics in the family.
func (p *ProposerSlashingTopicFamily) UnsubscribeAll() {
	p.unsubscribeAll()
}

func (p *ProposerSlashingTopicFamily) getFullTopicString() string {
	return p2p.ProposerSlashingSubnetTopic(p.nse.ForkDigest)
}

// Attester Slashing
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*AttesterSlashingTopicFamily)(nil)

type AttesterSlashingTopicFamily struct {
	*baseGossipsubTopicFamily
}

func NewAttesterSlashingTopicFamily(s *Service, nse params.NetworkScheduleEntry) *AttesterSlashingTopicFamily {
	a := &AttesterSlashingTopicFamily{}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateAttesterSlashing, s.attesterSlashingSubscriber, a)
	a.baseGossipsubTopicFamily = base
	return a
}

func (a *AttesterSlashingTopicFamily) Name() string {
	return "AttesterSlashingTopicFamily"
}

// Subscribe subscribes to the topic. Slot is ignored for this topic family.
func (a *AttesterSlashingTopicFamily) Subscribe() {
	a.subscribeToTopics([]string{a.getFullTopicString()})
}

// UnsubscribeAll unsubscribes from all topics in the family.
func (a *AttesterSlashingTopicFamily) UnsubscribeAll() {
	a.unsubscribeAll()
}

func (a *AttesterSlashingTopicFamily) getFullTopicString() string {
	return p2p.AttesterSlashingSubnetTopic(a.nse.ForkDigest)
}

// Sync Contribution and Proof (Altair+)
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*SyncContributionAndProofTopicFamily)(nil)

type SyncContributionAndProofTopicFamily struct{ *baseGossipsubTopicFamily }

func NewSyncContributionAndProofTopicFamily(s *Service, nse params.NetworkScheduleEntry) *SyncContributionAndProofTopicFamily {
	sc := &SyncContributionAndProofTopicFamily{}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateSyncContributionAndProof, s.syncContributionAndProofSubscriber, sc)
	sc.baseGossipsubTopicFamily = base
	return sc
}

func (sc *SyncContributionAndProofTopicFamily) Name() string {
	return "SyncContributionAndProofTopicFamily"
}

// Subscribe subscribes to the topic. Slot is ignored for this topic family.
func (sc *SyncContributionAndProofTopicFamily) Subscribe() {
	sc.subscribeToTopics([]string{sc.getFullTopicString()})
}

// UnsubscribeAll unsubscribes from all topics in the family.
func (sc *SyncContributionAndProofTopicFamily) UnsubscribeAll() {
	sc.unsubscribeAll()
}

func (sc *SyncContributionAndProofTopicFamily) getFullTopicString() string {
	return p2p.SyncContributionAndProofSubnetTopic(sc.nse.ForkDigest)
}

// Light Client Optimistic Update (Altair+)
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*LightClientOptimisticUpdateTopicFamily)(nil)

type LightClientOptimisticUpdateTopicFamily struct {
	*baseGossipsubTopicFamily
}

func NewLightClientOptimisticUpdateTopicFamily(s *Service, nse params.NetworkScheduleEntry) *LightClientOptimisticUpdateTopicFamily {
	l := &LightClientOptimisticUpdateTopicFamily{}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateLightClientOptimisticUpdate, noopHandler, l)
	l.baseGossipsubTopicFamily = base
	return l
}

func (l *LightClientOptimisticUpdateTopicFamily) Name() string {
	return "LightClientOptimisticUpdateTopicFamily"
}

// Subscribe subscribes to the topic. Slot is ignored for this topic family.
func (l *LightClientOptimisticUpdateTopicFamily) Subscribe() {
	l.subscribeToTopics([]string{l.getFullTopicString()})
}

// UnsubscribeAll unsubscribes from all topics in the family.
func (l *LightClientOptimisticUpdateTopicFamily) UnsubscribeAll() {
	l.unsubscribeAll()
}

func (l *LightClientOptimisticUpdateTopicFamily) getFullTopicString() string {
	return p2p.LcOptimisticToTopic(l.nse.ForkDigest)
}

// Light Client Finality Update (Altair+)
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*LightClientFinalityUpdateTopicFamily)(nil)

type LightClientFinalityUpdateTopicFamily struct {
	*baseGossipsubTopicFamily
}

func NewLightClientFinalityUpdateTopicFamily(s *Service, nse params.NetworkScheduleEntry) *LightClientFinalityUpdateTopicFamily {
	l := &LightClientFinalityUpdateTopicFamily{}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateLightClientFinalityUpdate, noopHandler, l)
	l.baseGossipsubTopicFamily = base
	return l
}

func (l *LightClientFinalityUpdateTopicFamily) Name() string {
	return "LightClientFinalityUpdateTopicFamily"
}

// Subscribe subscribes to the topic. Slot is ignored for this topic family.
func (l *LightClientFinalityUpdateTopicFamily) Subscribe() {
	l.subscribeToTopics([]string{l.getFullTopicString()})
}

// UnsubscribeAll unsubscribes from all topics in the family.
func (l *LightClientFinalityUpdateTopicFamily) UnsubscribeAll() {
	l.unsubscribeAll()
}

func (l *LightClientFinalityUpdateTopicFamily) getFullTopicString() string {
	return p2p.LcFinalityToTopic(l.nse.ForkDigest)
}

// BLS to Execution Change (Capella+)
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*BlsToExecutionChangeTopicFamily)(nil)

type BlsToExecutionChangeTopicFamily struct {
	*baseGossipsubTopicFamily
}

func NewBlsToExecutionChangeTopicFamily(s *Service, nse params.NetworkScheduleEntry) *BlsToExecutionChangeTopicFamily {
	b := &BlsToExecutionChangeTopicFamily{}
	base := newBaseGossipsubTopicFamily(s, nse, s.validateBlsToExecutionChange, s.blsToExecutionChangeSubscriber, b)
	b.baseGossipsubTopicFamily = base
	return b
}

func (b *BlsToExecutionChangeTopicFamily) Name() string {
	return "BlsToExecutionChangeTopicFamily"
}

// Subscribe subscribes to the topic. Slot is ignored for this topic family.
func (b *BlsToExecutionChangeTopicFamily) Subscribe() {
	b.subscribeToTopics([]string{b.getFullTopicString()})
}

// UnsubscribeAll unsubscribes from all topics in the family.
func (b *BlsToExecutionChangeTopicFamily) UnsubscribeAll() {
	b.unsubscribeAll()
}

func (b *BlsToExecutionChangeTopicFamily) getFullTopicString() string {
	return p2p.BlsToExecutionChangeSubnetTopic(b.nse.ForkDigest)
}
