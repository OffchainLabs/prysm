package blockchain

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	consensus_blocks "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/forkchoice"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// CachedHeadRoot returns the corresponding value from Forkchoice
func (s *Service) CachedHeadRoot() [32]byte {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.CachedHeadRoot()
}

// GetProposerHead returns the corresponding value from forkchoice
func (s *Service) GetProposerHead() [32]byte {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.GetProposerHead()
}

// SetForkChoiceGenesisTime sets the genesis time in Forkchoice
func (s *Service) SetForkChoiceGenesisTime(timestamp uint64) {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.cfg.ForkChoiceStore.SetGenesisTime(timestamp)
}

// HighestReceivedBlockSlot returns the corresponding value from forkchoice
func (s *Service) HighestReceivedBlockSlot() primitives.Slot {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.HighestReceivedBlockSlot()
}

// ReceivedBlocksLastEpoch returns the corresponding value from forkchoice
func (s *Service) ReceivedBlocksLastEpoch() (uint64, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.ReceivedBlocksLastEpoch()
}

// InsertNode is a wrapper for node insertion which is self locked
func (s *Service) InsertNode(ctx context.Context, st state.BeaconState, block consensus_blocks.ROBlock) error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.cfg.ForkChoiceStore.InsertNode(ctx, st, block)
}

// ForkChoiceDump returns the corresponding value from forkchoice
func (s *Service) ForkChoiceDump(ctx context.Context) (*forkchoice.Dump, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.ForkChoiceDump(ctx)
}

// NewSlot returns the corresponding value from forkchoice
func (s *Service) NewSlot(ctx context.Context, slot primitives.Slot) error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.cfg.ForkChoiceStore.NewSlot(ctx, slot)
}

// ProposerBoost wraps the corresponding method from forkchoice
func (s *Service) ProposerBoost() [32]byte {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.cfg.ForkChoiceStore.ProposerBoost()
}

// ChainHeads returns all possible chain heads (leaves of fork choice tree).
// Heads roots and heads slots are returned.
func (s *Service) ChainHeads() ([][32]byte, []primitives.Slot) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.Tips()
}

// UnrealizedJustifiedPayloadBlockHash returns unrealized justified payload block hash from forkchoice.
func (s *Service) UnrealizedJustifiedPayloadBlockHash() [32]byte {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.UnrealizedJustifiedPayloadBlockHash()
}

// FinalizedBlockHash returns finalized payload block hash from forkchoice.
func (s *Service) FinalizedBlockHash() [32]byte {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.FinalizedPayloadBlockHash()
}

// ParentRoot wraps a call to the corresponding method in forkchoice
func (s *Service) ParentRoot(root [32]byte) ([32]byte, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.ParentRoot(root)
}

// GetPTCVote wraps a call to the corresponding method in forkchoice and checks
// the currently syncing status
// Warning: this method will return the current PTC status regardless of
// timeliness. A client MUST call this method when about to submit a PTC
// attestation, that is exactly at the threshold to submit the attestation.
func (s *Service) GetPTCVote(root [32]byte) primitives.PTCStatus {
	s.cfg.ForkChoiceStore.RLock()
	f := s.cfg.ForkChoiceStore.GetPTCVote()
	s.cfg.ForkChoiceStore.RUnlock()
	if f != primitives.PAYLOAD_ABSENT {
		return f
	}
	f, isSyncing := s.payloadBeingSynced.isSyncing(root)
	if isSyncing {
		return f
	}
	return primitives.PAYLOAD_ABSENT
}

// insertPayloadEnvelope wraps a locked call to the corresponding method in
// forkchoice
func (s *Service) insertPayloadEnvelope(envelope interfaces.ROExecutionPayloadEnvelope) error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.cfg.ForkChoiceStore.InsertPayloadEnvelope(envelope)
}
