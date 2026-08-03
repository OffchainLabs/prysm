package node

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/genesis"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestValidateArchiveOrigin(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	require.NoError(t, genesis.Initialize(t.Context(), t.TempDir()))
	gvr := genesis.ValidatorsRoot()

	t.Run("epoch boundary is accepted", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetGenesisValidatorsRoot(gvr[:]))
		require.NoError(t, st.SetSlot(8192))
		require.NoError(t, validateArchiveOrigin(st))
	})

	t.Run("mid-epoch slot is rejected", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetGenesisValidatorsRoot(gvr[:]))
		require.NoError(t, st.SetSlot(8193))
		require.ErrorContains(t, "must be an epoch boundary", validateArchiveOrigin(st))
	})

	t.Run("wrong network is rejected", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetGenesisValidatorsRoot(make([]byte, 32)))
		require.NoError(t, st.SetSlot(8192))
		require.ErrorContains(t, "different network", validateArchiveOrigin(st))
	})

	t.Run("nil is rejected", func(t *testing.T) {
		require.ErrorContains(t, "nil", validateArchiveOrigin(nil))
	})
}
