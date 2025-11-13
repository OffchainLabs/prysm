package testing

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type getState func() (state.BeaconState, error)

func VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t *testing.T, factory getState) {
	st, err := factory()
	require.NoError(t, err)

	_, err = st.ValidatorAtIndexReadOnly(0)
	assert.ErrorContains(t, "index 0 out of bounds", err)
}
