package eth

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

// Proof type indices matching the zkvm ordering in the prover config.
const (
	ProofTypeRethOpenVM uint8 = iota + 3
	ProofTypeRethRisc0
	ProofTypeRethSP1
	ProofTypeRethZisk
)

var proofTypeNames = map[uint8]string{
	ProofTypeRethOpenVM: "reth-openvm",
	ProofTypeRethRisc0:  "reth-risc0",
	ProofTypeRethSP1:    "reth-sp1",
	ProofTypeRethZisk:   "reth-zisk",
}

// ProofTypeName returns the string name for a proof type index.
func ProofTypeName(proofType uint8) string {
	if name, ok := proofTypeNames[proofType]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", proofType)
}

// ProofTypeIndex returns the index for a proof type string name.
func ProofTypeIndex(proofType string) (uint8, error) {
	for idx, name := range proofTypeNames {
		if name == proofType {
			return idx, nil
		}
	}
	return 0, fmt.Errorf("unknown proof type: %s", proofType)
}

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
