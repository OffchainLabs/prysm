package eth

import "github.com/OffchainLabs/prysm/v7/encoding/bytesutil"

// Copy --
func (e *ExecutionProof) Copy() *ExecutionProof {
	if e == nil {
		return nil
	}

	return &ExecutionProof{
		ProofId:   e.ProofId,
		Slot:      e.Slot,
		BlockHash: bytesutil.SafeCopyBytes(e.BlockHash),
		BlockRoot: bytesutil.SafeCopyBytes(e.BlockRoot),
		ProofData: bytesutil.SafeCopyBytes(e.ProofData),
	}
}
