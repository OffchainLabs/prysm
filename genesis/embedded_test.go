package genesis

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/genesis/internal/embedded"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestEmbededGenesisDataMatchesMainnet(t *testing.T) {
	st, err := embedded.ByName(params.MainnetName)
	require.NoError(t, err)
	gvr := st.GenesisValidatorsRoot()

	data := embeddedGenesisData[params.MainnetName]
	require.DeepEqual(t, gvr, data.ValidatorsRoot[:])
	require.Equal(t, st.GenesisTime(), data.Time)
}
