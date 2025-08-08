package genesis_test

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/genesis"
	"github.com/OffchainLabs/prysm/v6/genesis/embedded"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestInitialize(t *testing.T) {
	require.NoError(t, genesis.Initialize(t.Context(), "testdata"))
	require.Equal(t, params.MainnetName, params.BeaconConfig().ConfigName)

}

func TestEmbeddedMainnetHardcodedValues(t *testing.T) {
	// Load the embedded mainnet state
	state, err := embedded.ByName(params.MainnetName)
	require.NoError(t, err)
	require.NotNil(t, state)

	// Verify hardcoded validators root matches the computed value from the state
	expectedValidatorsRoot := [32]byte{75, 54, 61, 185, 78, 40, 97, 32, 215, 110, 185, 5, 52, 15, 221, 78, 84, 191, 233, 240, 107, 243, 63, 246, 207, 90, 210, 127, 81, 27, 254, 149}
	actualValidatorsRoot := state.GenesisValidatorsRoot()
	require.Equal(t, expectedValidatorsRoot, [32]byte(actualValidatorsRoot), "hardcoded validators root does not match embedded state")

	// Verify hardcoded genesis time matches the computed value from the state
	expectedTime := time.Unix(1606824023, 0)
	actualTime := state.GenesisTime()
	require.Equal(t, expectedTime, actualTime, "hardcoded genesis time does not match embedded state")
}
