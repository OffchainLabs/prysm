package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/pkg/errors"
)

// ReceiveProof saves an execution proof to storage.
func (s *Service) ReceiveProof(proof blocks.VerifiedROSignedExecutionProof) error {
	if err := s.proofStorage.Save([]blocks.VerifiedROSignedExecutionProof{proof}); err != nil {
		return errors.Wrap(err, "save proof")
	}

	return nil
}
