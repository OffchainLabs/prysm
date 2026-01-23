package blockchain

import (
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// ReceiveProof saves an execution proof to storage.
func (s *Service) ReceiveProof(proof *ethpb.ExecutionProof) error {
	if err := s.proofStorage.Save([]*ethpb.ExecutionProof{proof}); err != nil {
		return errors.Wrap(err, "save proof")
	}

	return nil
}
