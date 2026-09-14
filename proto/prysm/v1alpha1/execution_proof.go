package eth

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

// ProofType identifies an immutable combination of proof system, guest program
// and version. Assignments are provisional and MUST NOT be reused.
//
// The values mirror the proof node's own discriminants (see
// github.com/eth-act/zkboost, crates/types/src/proof_type.rs), so that a
// ProofType on the wire needs no translation before it reaches the proof node.
type ProofType uint8

const (
	ProofTypeEthrexOpenVM ProofType = iota
	ProofTypeEthrexSP1
	ProofTypeEthrexZisk
	ProofTypeRethOpenVM
	ProofTypeRethSP1
	ProofTypeRethZisk
)

var proofTypeNames = map[ProofType]string{
	ProofTypeEthrexOpenVM: "ethrex-openvm",
	ProofTypeEthrexSP1:    "ethrex-sp1",
	ProofTypeEthrexZisk:   "ethrex-zisk",
	ProofTypeRethOpenVM:   "reth-openvm",
	ProofTypeRethSP1:      "reth-sp1",
	ProofTypeRethZisk:     "reth-zisk",
}

// String returns the proof node's name for this proof type.
func (p ProofType) String() string {
	if name, ok := proofTypeNames[p]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", uint8(p))
}

// Supported reports whether this proof type is one Prysm recognises.
func (p ProofType) Supported() bool {
	_, ok := proofTypeNames[p]
	return ok
}

// ParseProofType resolves a proof node proof type name to its identifier.
func ParseProofType(name string) (ProofType, error) {
	for proofType, n := range proofTypeNames {
		if n == name {
			return proofType, nil
		}
	}
	return 0, fmt.Errorf("unknown proof type %q", name)
}

// ProofTypeValue returns the envelope's proof type as a typed identifier. A
// malformed envelope, whose proof_type is not exactly one byte, yields an
// unsupported value that later validation rejects.
func (x *ExecutionProofEnvelope) ProofTypeValue() ProofType {
	if x == nil || len(x.ProofType) != 1 {
		return ProofType(0xff)
	}
	return ProofType(x.ProofType[0])
}

// Copy --
func (x *ExecutionProofEnvelope) Copy() *ExecutionProofEnvelope {
	if x == nil {
		return nil
	}
	return &ExecutionProofEnvelope{
		ProofData:       bytesutil.SafeCopyBytes(x.ProofData),
		ProofType:       bytesutil.SafeCopyBytes(x.ProofType),
		BeaconBlockRoot: bytesutil.SafeCopyBytes(x.BeaconBlockRoot),
	}
}

// Copy --
func (x *SignedExecutionProofEnvelope) Copy() *SignedExecutionProofEnvelope {
	if x == nil {
		return nil
	}
	return &SignedExecutionProofEnvelope{
		Message:        x.Message.Copy(),
		ValidatorIndex: x.ValidatorIndex,
		Signature:      bytesutil.SafeCopyBytes(x.Signature),
	}
}
