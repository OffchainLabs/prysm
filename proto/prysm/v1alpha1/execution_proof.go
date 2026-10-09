package eth

import (
	"fmt"
)

// ProofType identifies an immutable combination of proof system, guest program
// and version. Assignments are provisional and MUST NOT be reused.
//
// The specification supports proof types 1, 2 and 3 without naming them
// (get_supported_proof_types). Their names are the ones zkboost
// (github.com/eth-act/zkboost, crates/types/src/proof_type.rs), which produces
// the proofs, assigns to them. zkboost's other proof types are not supported.
type ProofType uint8

const (
	ProofTypeEthrexOpenVM ProofType = iota + 1
	ProofTypeEthrexSP1
	ProofTypeEthrexZisk
)

var proofTypeNames = map[ProofType]string{
	ProofTypeEthrexOpenVM: "ethrex-openvm",
	ProofTypeEthrexSP1:    "ethrex-sp1",
	ProofTypeEthrexZisk:   "ethrex-zisk",
}

// String returns the zkboost name of this proof type.
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
