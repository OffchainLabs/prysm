package structs

import (
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestBeaconStateHezeFromConsensus_EIP8015(t *testing.T) {
	eth1Fields := []string{"eth1_data", "eth1_data_votes", "eth1_deposit_index", "deposit_requests_start_index"}

	t.Run("heze drops the eth1 fields", func(t *testing.T) {
		state, err := util.NewBeaconStateHeze()
		require.NoError(t, err)
		converted, err := BeaconStateHezeFromConsensus(state)
		require.NoError(t, err)
		encoded, err := json.Marshal(converted)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &fields))
		for _, field := range eth1Fields {
			_, present := fields[field]
			require.Equal(t, false, present, field)
		}
	})

	t.Run("gloas keeps the eth1 fields", func(t *testing.T) {
		state, err := util.NewBeaconStateGloas()
		require.NoError(t, err)
		converted, err := BeaconStateGloasFromConsensus(state)
		require.NoError(t, err)
		encoded, err := json.Marshal(converted)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &fields))
		for _, field := range eth1Fields {
			_, present := fields[field]
			require.Equal(t, true, present, field)
		}
	})
}
