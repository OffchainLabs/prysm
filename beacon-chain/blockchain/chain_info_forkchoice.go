package blockchain

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	consensus_blocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/forkchoice"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
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
func (s *Service) SetForkChoiceGenesisTime(timestamp time.Time) {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.cfg.ForkChoiceStore.SetGenesisTime(timestamp)
}

// HighestReceivedBlockSlotRoot returns the corresponding value from forkchoice
func (s *Service) HighestReceivedBlockSlotRoot() (primitives.Slot, [32]byte) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.HighestReceivedBlockSlotRoot()
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

// hashForGenesisBlock returns the right hash for the genesis block
func (s *Service) hashForGenesisBlock(ctx context.Context, root [32]byte) ([]byte, error) {
	genRoot, err := s.cfg.BeaconDB.GenesisBlockRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get genesis block root")
	}
	if root != genRoot {
		return nil, errNotGenesisRoot
	}
	st, err := s.cfg.BeaconDB.GenesisState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get genesis state")
	}
	if st.Version() < version.Bellatrix {
		return nil, nil
	}
	if st.Version() >= version.EPBS {
		header, err := st.LatestExecutionPayloadHeaderEPBS()
		if err != nil {
			return nil, errors.Wrap(err, "could not get latest execution payload header")
		}
		return bytesutil.SafeCopyBytes(header.BlockHash), nil
	}
	header, err := st.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest execution payload header")
	}
	return bytesutil.SafeCopyBytes(header.BlockHash()), nil
}

// DependentRoot wraps the corresponding method in forkchoice
func (s *Service) DependentRoot(epoch primitives.Epoch) ([32]byte, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.DependentRoot(epoch)
}

// HashForBlockRoot wraps a call to the corresponding method in forkchoice. If the hash is older it will grab it from DB
func (s *Service) HashForBlockRoot(ctx context.Context, root [32]byte) ([]byte, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	hash := s.cfg.ForkChoiceStore.HashForBlockRoot(root)
	if hash != [32]byte{} {
		return hash[:], nil
	}
	blk, err := s.cfg.BeaconDB.Block(ctx, root)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get block from DB and forkchoice")
	}
	// check for genesis block hash
	genHash, err := s.hashForGenesisBlock(ctx, root)
	if err == nil {
		return genHash, nil
	}
	if !errors.Is(err, errNotGenesisRoot) {
		return nil, errors.Wrap(err, "failed to get genesis block hash")
	}
	if blk.Version() < version.EPBS {
		return nil, errors.New("block version is too old")
	}
	sh, err := blk.Block().Body().SignedExecutionPayloadHeader()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get signed execution payload header")
	}
	h, err := sh.Header()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get execution payload header")
	}
	hash = h.BlockHash()
	return hash[:], nil
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
