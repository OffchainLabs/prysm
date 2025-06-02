package state_native

import (
	"slices"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
)

// ProposerLookahead is a non-mutating call to the beacon state which returns a slice of
// validator indices that hold the  proposers in the next few slots.
func (b *BeaconState) ProposerLookahead() ([]primitives.ValidatorIndex, error) {
	if b.version < version.Fulu {
		return nil, errNotSupported("ProposerLookahead", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return slices.Clone(b.proposerLookahead), nil
}
