package sync

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/config/params"
)

// Blocks
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*BlockTopicFamily)(nil)

type BlockTopicFamily struct {
	baseGossipsubTopicFamily
}

func NewBlockTopicFamily(s *Service, nse params.NetworkScheduleEntry) *BlockTopicFamily {
	return &BlockTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix()},
	}
}

func (b *BlockTopicFamily) Validator() wrappedVal {
	return b.syncService.validateBeaconBlockPubSub
}

func (b *BlockTopicFamily) Handler() subHandler {
	return b.syncService.beaconBlockSubscriber
}

func (b *BlockTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.BlockSubnetTopicFormat, b.nse.ForkDigest) + b.protocolSuffix
}

func (b *BlockTopicFamily) Subscribe() {
	b.syncService.subscribe(b)
}

func (b *BlockTopicFamily) Unsubscribe() {
	b.syncService.unSubscribeFromTopic(b.GetFullTopicString())
}

// Aggregate and Proof
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*AggregateAndProofTopicFamily)(nil)

type AggregateAndProofTopicFamily struct {
	baseGossipsubTopicFamily
}

func NewAggregateAndProofTopicFamily(s *Service, nse params.NetworkScheduleEntry) *AggregateAndProofTopicFamily {
	return &AggregateAndProofTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix()},
	}
}

func (a *AggregateAndProofTopicFamily) Validator() wrappedVal {
	return a.syncService.validateAggregateAndProof
}

func (a *AggregateAndProofTopicFamily) Handler() subHandler {
	return a.syncService.beaconAggregateProofSubscriber
}

func (a *AggregateAndProofTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.AggregateAndProofSubnetTopicFormat, a.nse.ForkDigest) + a.protocolSuffix
}

func (a *AggregateAndProofTopicFamily) Subscribe() {
	a.syncService.subscribe(a)
}

func (a *AggregateAndProofTopicFamily) Unsubscribe() {
	a.syncService.unSubscribeFromTopic(a.GetFullTopicString())
}

// Voluntary Exit
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*VoluntaryExitTopicFamily)(nil)

type VoluntaryExitTopicFamily struct {
	baseGossipsubTopicFamily
}

func NewVoluntaryExitTopicFamily(s *Service, nse params.NetworkScheduleEntry) *VoluntaryExitTopicFamily {
	return &VoluntaryExitTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix()},
	}
}

func (v *VoluntaryExitTopicFamily) Validator() wrappedVal {
	return v.syncService.validateVoluntaryExit
}

func (v *VoluntaryExitTopicFamily) Handler() subHandler {
	return v.syncService.voluntaryExitSubscriber
}

func (v *VoluntaryExitTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.ExitSubnetTopicFormat, v.nse.ForkDigest) + v.protocolSuffix
}

func (v *VoluntaryExitTopicFamily) Subscribe() {
	v.syncService.subscribe(v)
}

func (v *VoluntaryExitTopicFamily) Unsubscribe() {
	v.syncService.unSubscribeFromTopic(v.GetFullTopicString())
}

// Proposer Slashing
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*ProposerSlashingTopicFamily)(nil)

type ProposerSlashingTopicFamily struct {
	baseGossipsubTopicFamily
}

func NewProposerSlashingTopicFamily(s *Service, nse params.NetworkScheduleEntry) *ProposerSlashingTopicFamily {
	return &ProposerSlashingTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix()},
	}
}

func (p *ProposerSlashingTopicFamily) Validator() wrappedVal {
	return p.syncService.validateProposerSlashing
}

func (p *ProposerSlashingTopicFamily) Handler() subHandler {
	return p.syncService.proposerSlashingSubscriber
}

func (p *ProposerSlashingTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.ProposerSlashingSubnetTopicFormat, p.nse.ForkDigest) + p.protocolSuffix
}

func (p *ProposerSlashingTopicFamily) Subscribe() {
	p.syncService.subscribe(p)
}

func (p *ProposerSlashingTopicFamily) Unsubscribe() {
	p.syncService.unSubscribeFromTopic(p.GetFullTopicString())
}

// Attester Slashing
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*AttesterSlashingTopicFamily)(nil)

type AttesterSlashingTopicFamily struct {
	baseGossipsubTopicFamily
}

func NewAttesterSlashingTopicFamily(s *Service, nse params.NetworkScheduleEntry) *AttesterSlashingTopicFamily {
	return &AttesterSlashingTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix()},
	}
}

func (a *AttesterSlashingTopicFamily) Validator() wrappedVal {
	return a.syncService.validateAttesterSlashing
}

func (a *AttesterSlashingTopicFamily) Handler() subHandler {
	return a.syncService.attesterSlashingSubscriber
}

func (a *AttesterSlashingTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.AttesterSlashingSubnetTopicFormat, a.nse.ForkDigest) + a.protocolSuffix
}

// TODO: Do we really need to spawn go-routines here ?
func (a *AttesterSlashingTopicFamily) Subscribe() {
	a.syncService.subscribe(a)
}

func (a *AttesterSlashingTopicFamily) Unsubscribe() {
	a.syncService.unSubscribeFromTopic(a.GetFullTopicString())
}

// Sync Contribution and Proof (Altair+)
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*SyncContributionAndProofTopicFamily)(nil)

type SyncContributionAndProofTopicFamily struct{ baseGossipsubTopicFamily }

func NewSyncContributionAndProofTopicFamily(s *Service, nse params.NetworkScheduleEntry) *SyncContributionAndProofTopicFamily {
	return &SyncContributionAndProofTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix()},
	}
}

func (sc *SyncContributionAndProofTopicFamily) Validator() wrappedVal {
	return sc.syncService.validateSyncContributionAndProof
}

func (sc *SyncContributionAndProofTopicFamily) Handler() subHandler {
	return sc.syncService.syncContributionAndProofSubscriber
}

func (sc *SyncContributionAndProofTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.SyncContributionAndProofSubnetTopicFormat, sc.nse.ForkDigest) + sc.protocolSuffix
}

func (sc *SyncContributionAndProofTopicFamily) Subscribe() {
	sc.syncService.subscribe(sc)
}

func (sc *SyncContributionAndProofTopicFamily) Unsubscribe() {
	sc.syncService.unSubscribeFromTopic(sc.GetFullTopicString())
}

// Light Client Optimistic Update (Altair+)
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*LightClientOptimisticUpdateTopicFamily)(nil)

type LightClientOptimisticUpdateTopicFamily struct {
	baseGossipsubTopicFamily
}

func NewLightClientOptimisticUpdateTopicFamily(s *Service, nse params.NetworkScheduleEntry) *LightClientOptimisticUpdateTopicFamily {
	return &LightClientOptimisticUpdateTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix()},
	}
}

func (l *LightClientOptimisticUpdateTopicFamily) Validator() wrappedVal {
	return l.syncService.validateLightClientOptimisticUpdate
}

func (l *LightClientOptimisticUpdateTopicFamily) Handler() subHandler {
	return noopHandler
}

func (l *LightClientOptimisticUpdateTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.LightClientOptimisticUpdateTopicFormat, l.nse.ForkDigest) + l.protocolSuffix
}

func (l *LightClientOptimisticUpdateTopicFamily) Subscribe() {
	l.syncService.subscribe(l)
}

func (l *LightClientOptimisticUpdateTopicFamily) Unsubscribe() {
	l.syncService.unSubscribeFromTopic(l.GetFullTopicString())
}

// Light Client Finality Update (Altair+)
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*LightClientFinalityUpdateTopicFamily)(nil)

type LightClientFinalityUpdateTopicFamily struct {
	baseGossipsubTopicFamily
}

func NewLightClientFinalityUpdateTopicFamily(s *Service, nse params.NetworkScheduleEntry) *LightClientFinalityUpdateTopicFamily {
	return &LightClientFinalityUpdateTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix()},
	}
}

func (l *LightClientFinalityUpdateTopicFamily) Validator() wrappedVal {
	return l.syncService.validateLightClientFinalityUpdate
}

func (l *LightClientFinalityUpdateTopicFamily) Handler() subHandler {
	return noopHandler
}

func (l *LightClientFinalityUpdateTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.LightClientFinalityUpdateTopicFormat, l.nse.ForkDigest) + l.protocolSuffix
}

func (l *LightClientFinalityUpdateTopicFamily) Subscribe() {
	l.syncService.subscribe(l)
}
func (l *LightClientFinalityUpdateTopicFamily) Unsubscribe() {
	l.syncService.unSubscribeFromTopic(l.GetFullTopicString())
}

// BLS to Execution Change (Capella+)
var _ GossipsubTopicFamilyWithoutDynamicSubnets = (*BlsToExecutionChangeTopicFamily)(nil)

type BlsToExecutionChangeTopicFamily struct {
	baseGossipsubTopicFamily
}

func NewBlsToExecutionChangeTopicFamily(s *Service, nse params.NetworkScheduleEntry) *BlsToExecutionChangeTopicFamily {
	return &BlsToExecutionChangeTopicFamily{
		baseGossipsubTopicFamily{
			syncService:    s,
			nse:            nse,
			protocolSuffix: s.cfg.p2p.Encoding().ProtocolSuffix()},
	}
}

func (b *BlsToExecutionChangeTopicFamily) Validator() wrappedVal {
	return b.syncService.validateBlsToExecutionChange
}

func (b *BlsToExecutionChangeTopicFamily) Handler() subHandler {
	return b.syncService.blsToExecutionChangeSubscriber
}

func (b *BlsToExecutionChangeTopicFamily) GetFullTopicString() string {
	return fmt.Sprintf(p2p.BlsToExecutionChangeSubnetTopicFormat, b.nse.ForkDigest) + b.protocolSuffix
}

func (b *BlsToExecutionChangeTopicFamily) Subscribe() {
	b.syncService.subscribe(b)
}

func (b *BlsToExecutionChangeTopicFamily) Unsubscribe() {
	b.syncService.unSubscribeFromTopic(b.GetFullTopicString())
}
