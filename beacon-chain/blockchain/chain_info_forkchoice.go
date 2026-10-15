package blockchain

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/confirmation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensus_blocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/forkchoice"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

var (
	// ErrInvalidDependentRoot is returned when a root is not a possible dependent block, as in is_valid_dependent_root.
	ErrInvalidDependentRoot = errors.New("dependent block is not a possible dependent block")
	// ErrDependentStateUnavailable is returned when no head or cached state can give the lookahead committees cheaply.
	ErrDependentStateUnavailable = errors.New("dependent state is not available")
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

// HighestReceivedBlockSlot returns the corresponding value from forkchoice
func (s *Service) HighestReceivedBlockSlot() primitives.Slot {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.HighestReceivedBlockSlot()
}

// HighestReceivedBlockRoot returns the corresponding value from forkchoice
func (s *Service) HighestReceivedBlockRoot() [32]byte {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.HighestReceivedBlockRoot()
}

// BlockHash returns the execution payload block hash for the given beacon block root from forkchoice.
func (s *Service) BlockHash(root [32]byte) ([32]byte, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.BlockHash(root)
}

// BuilderPubkey returns the pubkey of the builder committed in the given block's bid, nil for self-built blocks.
func (s *Service) BuilderPubkey(root [32]byte) (*[fieldparams.BLSPubkeyLength]byte, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.BuilderPubkey(root)
}

// HasPayloadBlockHash reports whether blockHash is an available payload parent at root.
func (s *Service) HasPayloadBlockHash(root, blockHash [32]byte) bool {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.HasPayloadBlockHash(root, blockHash)
}

// GasLimit returns the gas limit of the payload with the given block hash as seen from the given beacon block root:
// the block's own payload or the parent payload it builds on.
func (s *Service) GasLimit(root, blockHash [32]byte) (uint64, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.GasLimit(root, blockHash)
}

// HasNode returns the corresponding value from forkchoice
func (s *Service) HasNode(root [32]byte) bool {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.HasNode(root)
}

// HasFullNode returns the corresponding value from forkchoice
func (s *Service) HasFullNode(root [32]byte) bool {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.HasFullNode(root)
}

// FullBeatsEmpty returns whether forkchoice would select the full payload variant for the given root.
func (s *Service) FullBeatsEmpty(root [32]byte) bool {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.FullBeatsEmpty(root)
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

// InsertPayload is a wrapper for payload insertion which is self locked
func (s *Service) InsertPayload(pe interfaces.ROExecutionPayloadEnvelope) error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.cfg.ForkChoiceStore.InsertPayload(pe)
}

// InsertPayloadWithInclusionList is a self-locked wrapper that inserts the payload and records its inclusion list satisfaction.
func (s *Service) InsertPayloadWithInclusionList(pe interfaces.ROExecutionPayloadEnvelope, ilSatisfied bool) error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	if err := s.cfg.ForkChoiceStore.InsertPayload(pe); err != nil {
		return err
	}

	// Note: Pre-Heze fork, this setter has no effect as inclusion list satisfaction is not tracked.
	return s.cfg.ForkChoiceStore.SetPayloadInclusionListSatisfied(pe.BeaconBlockRoot(), ilSatisfied)
}

// ForkChoiceDump returns the corresponding value from forkchoice
func (s *Service) ForkChoiceDump(ctx context.Context) (*forkchoice.Dump, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.ForkChoiceDump(ctx)
}

// ForkChoiceDumpV2 returns the corresponding value from forkchoice
func (s *Service) ForkChoiceDumpV2(ctx context.Context) (*forkchoice.DumpV2, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.ForkChoiceDumpV2(ctx)
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

// FCR returns the fast confirmation rule instance, or nil if FCR is not enabled.
func (s *Service) FCR() *confirmation.FastConfirmationRule {
	return s.fcr
}

// ChainHeads returns all possible chain heads (leaves of fork choice tree).
// Heads roots and heads slots are returned.
func (s *Service) ChainHeads() ([][32]byte, []primitives.Slot) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.Tips()
}

// isValidDependentRoot reports whether root is the head or the dependent root for dependentEpoch of another chain tip, as in is_valid_dependent_root.
func (s *Service) isValidDependentRoot(root [32]byte, dependentEpoch primitives.Epoch) bool {
	if headRoot, _ := s.HeadRootAndFull(); headRoot == root {
		return true
	}
	tips, _ := s.ChainHeads()
	for _, tip := range tips {
		if tip == root {
			continue
		}
		depRoot, err := s.DependentRootForEpoch(tip, dependentEpoch)
		if err == nil && depRoot == root {
			return true
		}
	}
	return false
}

// LookaheadState returns a state with the committees for dependentEpoch on the branch of dependentRoot,
// advanced to the start of dependentEpoch.
func (s *Service) LookaheadState(ctx context.Context, dependentRoot [32]byte, dependentEpoch primitives.Epoch) (state.ReadOnlyBeaconState, error) {
	st, stateRoot, err := s.dependentState(ctx, dependentRoot, dependentEpoch)
	if err != nil {
		return nil, err
	}
	lookaheadStart, err := slots.EpochStart(dependentEpoch)
	if err != nil {
		return nil, errors.Wrap(err, "lookahead start slot")
	}
	st, err = transition.ProcessSlotsIfNeeded(ctx, st, stateRoot[:], lookaheadStart)
	if err != nil {
		return nil, errors.Wrap(err, "process slots to the lookahead start slot")
	}
	return st, nil
}

// DependentState returns the state that LookaheadState starts from, before any slot processing.
// A caller can check a signature against it before it pays for the slot processing.
func (s *Service) DependentState(ctx context.Context, dependentRoot [32]byte, dependentEpoch primitives.Epoch) (state.ReadOnlyBeaconState, error) {
	st, _, err := s.dependentState(ctx, dependentRoot, dependentEpoch)
	return st, err
}

// dependentState returns the head state or the cached state of dependentRoot, and its root. It does no regeneration.
func (s *Service) dependentState(ctx context.Context, dependentRoot [32]byte, dependentEpoch primitives.Epoch) (state.ReadOnlyBeaconState, [32]byte, error) {
	headRoot, _ := s.HeadRootAndFull()
	headDependentRoot, err := s.DependentRootForEpoch(headRoot, dependentEpoch)

	var st state.ReadOnlyBeaconState
	stateRoot := headRoot
	if err == nil && headDependentRoot == dependentRoot {
		st, err = s.HeadStateReadOnly(ctx)
		if err != nil {
			return nil, [32]byte{}, errors.Wrap(err, "head state")
		}
	} else {
		if !s.isValidDependentRoot(dependentRoot, dependentEpoch) {
			return nil, [32]byte{}, errors.Wrapf(ErrInvalidDependentRoot, "%#x", dependentRoot)
		}
		st = s.cfg.StateGen.StateByRootIfCachedNoCopy(dependentRoot)
		stateRoot = dependentRoot
	}
	if st == nil || st.IsNil() {
		return nil, [32]byte{}, errors.Wrapf(ErrDependentStateUnavailable, "%#x", dependentRoot)
	}
	if stateEpoch := slots.ToEpoch(st.Slot()); stateEpoch+1 < dependentEpoch {
		return nil, [32]byte{}, errors.Wrapf(ErrDependentStateUnavailable, "state epoch %d is more than one epoch before lookahead epoch %d", stateEpoch, dependentEpoch)
	}
	return st, stateRoot, nil
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
	if st.Version() >= version.Gloas {
		h, err := st.LatestBlockHash()
		if err != nil {
			return nil, errors.Wrap(err, "could not get latest block hash")
		}
		return bytesutil.SafeCopyBytes(h[:]), nil
	}
	header, err := st.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest execution payload header")
	}
	return bytesutil.SafeCopyBytes(header.BlockHash()), nil
}

// CanonicalNodeAtSlot wraps the corresponding method in forkchoice
func (s *Service) CanonicalNodeAtSlot(slot primitives.Slot) ([32]byte, bool) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.CanonicalNodeAtSlot(slot)
}

// DependentRoot wraps the corresponding method in forkchoice
func (s *Service) DependentRoot(epoch primitives.Epoch) ([32]byte, error) {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	return s.cfg.ForkChoiceStore.DependentRoot(epoch)
}
