package state_native

import (
	"errors"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
)

// SetProposerLookahead is a mutating call to the beacon state which sets the proposer lookahead
func (b *BeaconState) SetProposerLookahead(lookahead []primitives.ValidatorIndex) error {
	if b.version < version.Fulu {
		return errNotSupported("SetProposerLookahead", b.version)
	}
	if len(lookahead) != int((params.BeaconConfig().MinSeedLookahead+1))*int(params.BeaconConfig().SlotsPerEpoch) {
		return errors.New("invalid size for proposer lookahead")
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.sharedFieldReferences[types.ProposerLookahead].MinusRef()
	b.sharedFieldReferences[types.ProposerLookahead] = stateutil.NewRef(1)

	b.proposerLookahead = lookahead

	b.markFieldAsDirty(types.ProposerLookahead)
	return nil
}
