package util

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/pkg/errors"
)

// DeterministicGenesisStateGloas returns a genesis state in Gloas format made
// using the deterministic deposits.
func DeterministicGenesisStateGloas(t testing.TB, numValidators uint64) (state.BeaconState, []bls.SecretKey) {
	t.Helper()

	fuluState, privKeys := DeterministicGenesisStateFulu(t, numValidators)
	beaconState, err := gloas.UpgradeToGloas(fuluState)
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to upgrade genesis beacon state of %d validators to gloas", numValidators))
	}
	resetCache()
	return beaconState, privKeys
}
