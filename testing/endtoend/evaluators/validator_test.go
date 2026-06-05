package evaluators

import (
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestPreviousEpochParticipationFulu(t *testing.T) {
	data, err := json.Marshal(&structs.BeaconStateFulu{
		PreviousEpochParticipation: []string{"1", "2", "3"},
	})
	require.NoError(t, err)

	got, err := previousEpochParticipation(version.String(version.Fulu), data)
	require.NoError(t, err)
	require.DeepEqual(t, []string{"1", "2", "3"}, got)
}

func TestPreviousEpochParticipationUnknownVersion(t *testing.T) {
	_, err := previousEpochParticipation("bogus", json.RawMessage(`{}`))
	require.ErrorContains(t, "unrecognized version bogus", err)
}
