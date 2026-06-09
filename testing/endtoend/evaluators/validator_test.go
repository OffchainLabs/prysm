package evaluators

import (
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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

func TestSkipStartupSyncParticipationSlot(t *testing.T) {
	tests := []struct {
		name string
		slot primitives.Slot
		want bool
	}{
		{name: "slot 2 is startup", slot: 2, want: true},
		{name: "slot 3 is startup", slot: 3, want: true},
		{name: "slot 4 is checked", slot: 4, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, skipStartupSyncParticipationSlot(tt.slot))
		})
	}
}
