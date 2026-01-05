package execproof

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// ProofKey uniquely identifies an execution proof by slot and proof type.
type ProofKey struct {
	Slot    primitives.Slot
	ProofId primitives.ExecutionProofId
}

// String returns a string representation for logging.
func (k ProofKey) String() string {
	return fmt.Sprintf("slot=%d,proofId=%d", k.Slot, k.ProofId)
}
