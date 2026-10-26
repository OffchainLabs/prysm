package eth

import (
	"fmt"
)

// ProofType identifies an immutable combination of proof system, guest program
// and version. Assignments are provisional and MUST NOT be reused.
//
// The specification supports proof types 1, 2 and 3 without naming them
// (get_supported_proof_types). Prysm also supports proof types 4 to 7, as
// Lighthouse (eth-act/lighthouse, branch optional-proofs) does, although the
// specification rejects them. Their names are the ones zkboost
// (github.com/eth-act/zkboost, crates/types/src/proof_type.rs), which produces
// the proofs, assigns to them.
type ProofType uint8

const (
	ProofTypeEthrexOpenVM ProofType = iota + 1
	ProofTypeEthrexSP1
	ProofTypeEthrexZisk
	ProofTypeRethOpenVM
	ProofTypeRethSP1
	ProofTypeRethZisk
	ProofTypeZesuZisk
)

var proofTypeNames = map[ProofType]string{
	ProofTypeEthrexOpenVM: "ethrex-openvm",
	ProofTypeEthrexSP1:    "ethrex-sp1",
	ProofTypeEthrexZisk:   "ethrex-zisk",
	ProofTypeRethOpenVM:   "reth-openvm",
	ProofTypeRethSP1:      "reth-sp1",
	ProofTypeRethZisk:     "reth-zisk",
	ProofTypeZesuZisk:     "zesu-zisk",
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
