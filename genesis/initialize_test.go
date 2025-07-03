package genesis_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/genesis"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestInitialize(t *testing.T) {
	require.NoError(t, genesis.Initialize(t.Context(), "testdata"))
	require.Equal(t, params.MainnetName, params.BeaconConfig().ConfigName)

}
