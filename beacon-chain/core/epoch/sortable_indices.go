package epoch

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// sortableIndices implements the Sort interface to sort newly activated validator indices
// by activation epoch and by index number.
type sortableIndices struct {
	indices []primitives.ValidatorIndex
	state   state.ReadOnlyValidators
}

// Len is the number of elements in the collection.
func (s sortableIndices) Len() int { return len(s.indices) }

// Swap swaps the elements with indexes i and j.
func (s sortableIndices) Swap(i, j int) { s.indices[i], s.indices[j] = s.indices[j], s.indices[i] }

// Less reports whether the element with index i must sort before the element with index j.
func (s sortableIndices) Less(i, j int) bool {
	vi, erri := s.state.ValidatorAtIndexReadOnly(s.indices[i])
	vj, errj := s.state.ValidatorAtIndexReadOnly(s.indices[j])

	if erri != nil || errj != nil {
		return false
	}

	a, b := vi.ActivationEligibilityEpoch(), vj.ActivationEligibilityEpoch()
	if a == b {
		return s.indices[i] < s.indices[j]
	}
	return a < b
}
