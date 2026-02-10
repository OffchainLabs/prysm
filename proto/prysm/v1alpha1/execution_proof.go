package eth

import "github.com/OffchainLabs/prysm/v7/encoding/bytesutil"

// Copy --
func (e *ExecutionProof) Copy() *ExecutionProof {
	if e == nil {
		return nil
	}

	return &ExecutionProof{
		ProofData: bytesutil.SafeCopyBytes(e.ProofData),
		ProofType: e.ProofType,
		PublicInput: &PublicInput{
			NewPayloadRequestRoot: bytesutil.SafeCopyBytes(e.PublicInput.NewPayloadRequestRoot),
		},
	}
}
