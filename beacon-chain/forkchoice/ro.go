package forkchoice

import (
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// ROForkChoice is an implementation of forkchoice.Getter which calls `Rlock`/`RUnlock`
// around a delegated method call to the underlying Getter implementation.
type ROForkChoice struct {
	getter FastGetter
	l      RLocker
}

var _ FastGetter = &ROForkChoice{}

// ROWrappable represents the subset of ForkChoicer a type needs to support
// in order for ROForkChoice to wrap it. This simplifies the creation of a mock
// type that can be used to assert that all of the wrapped methods are correctly
// called between mutex acquire/release.
type ROWrappable interface {
	RLocker
	FastGetter
}

// NewROForkChoice returns an ROForkChoice that delegates forkchoice.Getter calls to the
// given value after first using its Locker methods to make sure it is correctly locked.
func NewROForkChoice(w ROWrappable) *ROForkChoice {
	return &ROForkChoice{getter: w, l: w}
}

// HasFullNode delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) HasFullNode(root [32]byte) bool {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.HasFullNode(root)
}

// IsFullNode delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) IsFullNode(root [32]byte) bool {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.IsFullNode(root)
}

// HasNode delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) HasNode(root [32]byte) bool {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.HasNode(root)
}

// ProposerBoost delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) ProposerBoost() [fieldparams.RootLength]byte {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.ProposerBoost()
}

// IsCanonical delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) IsCanonical(root [32]byte) bool {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.IsCanonical(root)
}

// FinalizedCheckpoint delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) FinalizedCheckpoint() *forkchoicetypes.Checkpoint {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.FinalizedCheckpoint()
}

// IsViableForCheckpoint delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) IsViableForCheckpoint(cp *forkchoicetypes.Checkpoint) (bool, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.IsViableForCheckpoint(cp)
}

// FinalizedPayloadBlockHash delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) FinalizedPayloadBlockHash() [32]byte {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.FinalizedPayloadBlockHash()
}

// JustifiedCheckpoint delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) JustifiedCheckpoint() *forkchoicetypes.Checkpoint {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.JustifiedCheckpoint()
}

// PreviousJustifiedCheckpoint delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) PreviousJustifiedCheckpoint() *forkchoicetypes.Checkpoint {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.PreviousJustifiedCheckpoint()
}

// JustifiedPayloadBlockHash delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) JustifiedPayloadBlockHash() [32]byte {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.JustifiedPayloadBlockHash()
}

// UnrealizedJustifiedPayloadBlockHash delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) UnrealizedJustifiedPayloadBlockHash() [32]byte {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.UnrealizedJustifiedPayloadBlockHash()
}

// NodeCount delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) NodeCount() int {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.NodeCount()
}

// HighestReceivedBlockSlot delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) HighestReceivedBlockSlot() primitives.Slot {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.HighestReceivedBlockSlot()
}

// HighestReceivedBlockRoot delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) HighestReceivedBlockRoot() [32]byte {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.HighestReceivedBlockRoot()
}

// ReceivedBlocksLastEpoch delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) ReceivedBlocksLastEpoch() (uint64, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.ReceivedBlocksLastEpoch()
}

// Weight delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) Weight(root [32]byte) (uint64, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.Weight(root)
}

// ConsensusNodeWeight delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) ConsensusNodeWeight(root [32]byte) (uint64, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.ConsensusNodeWeight(root)
}

// PayloadWeights delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) PayloadWeights(root [32]byte) (uint64, uint64, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.PayloadWeights(root)
}

// IsOptimistic delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) IsOptimistic(root [32]byte) (bool, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.IsOptimistic(root)
}

// ShouldOverrideFCU delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) ShouldOverrideFCU() bool {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.ShouldOverrideFCU()
}

// Slot delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) Slot(root [32]byte) (primitives.Slot, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.Slot(root)
}

// DependentRoot delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) DependentRoot(epoch primitives.Epoch) ([32]byte, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.DependentRoot(epoch)
}

// DependentRootForEpoch delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) DependentRootForEpoch(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.DependentRootForEpoch(root, epoch)
}

// TargetRootForEpoch delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) TargetRootForEpoch(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.TargetRootForEpoch(root, epoch)
}

// ParentRoot delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) ParentRoot(root [32]byte) ([32]byte, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.ParentRoot(root)
}

// BlockHash delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) BlockHash(root [32]byte) ([32]byte, error) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.BlockHash(root)
}

// CanonicalNodeAtSlot delegates to the underlying forkchoice call, under a lock.
func (ro *ROForkChoice) CanonicalNodeAtSlot(slot primitives.Slot) ([32]byte, bool) {
	ro.l.RLock()
	defer ro.l.RUnlock()
	return ro.getter.CanonicalNodeAtSlot(slot)
}
