package blockchain

import (
	"fmt"

	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/pkg/errors"
)

// ReceiveProof saves an execution proof to storage and, when enough proofs
// have been collected, marks the block's forkchoice node accordingly.
func (s *Service) ReceiveProof(proof blocks.VerifiedROSignedExecutionProof) error {
	if err := s.proofStorage.Save([]blocks.VerifiedROSignedExecutionProof{proof}); err != nil {
		return fmt.Errorf("save proof: %w", err)
	}

	if !features.Get().EnableZkvm {
		return nil
	}

	required := params.BeaconConfig().MinProofsRequired
	summary := s.proofStorage.Summary(proof.BlockRoot())
	if uint64(summary.Count()) < required {
		return nil
	}

	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()

	if err := s.cfg.ForkChoiceStore.MarkHasEnoughProofs(s.ctx, proof.BlockRoot()); err != nil {
		if errors.Is(err, doublylinkedtree.ErrNilNode) {
			return nil // proof arrived before block
		}
		return fmt.Errorf("mark has enough proofs: %w", err)
	}

	// Update head's cached optimistic status.
	s.headLock.Lock()
	defer s.headLock.Unlock()
	if s.head == nil {
		return nil
	}

	headOptimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(s.head.root)
	if err != nil {
		return fmt.Errorf("is optimistic: %w", err)
	}

	s.head.optimistic = headOptimistic

	return nil
}
