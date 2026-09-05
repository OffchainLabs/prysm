package confirmation

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// EquivocationScorer computes equivocation score for a slot range.
// This abstracts the spec's get_equivocation_score which checks committee membership.
type EquivocationScorer func(startSlot, endSlot primitives.Slot) uint64

// FFGStateInfo holds the effective balances and total active balance of a balance source state.
type FFGStateInfo struct {
	TotalActiveBalance uint64
	// Effective balances by validator index, zero for inactive or slashed validators.
	Balances []uint64
	// Effective balances of active but slashed validators, the equivocation score must count them.
	SlashedBalances map[primitives.ValidatorIndex]uint64
}
