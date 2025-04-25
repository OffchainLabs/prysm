package epoch

import (
	"sort"
	"testing"

	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/google/go-cmp/cmp"
)

func TestSortableIndices(t *testing.T) {
	st, err := state_native.InitializeFromProtoPhase0(&eth.BeaconState{
		Validators: []*eth.Validator{
			{ActivationEligibilityEpoch: 0},
			{ActivationEligibilityEpoch: 5},
			{ActivationEligibilityEpoch: 4},
			{ActivationEligibilityEpoch: 4},
			{ActivationEligibilityEpoch: 2},
			{ActivationEligibilityEpoch: 1},
		},
	})
	require.NoError(t, err)

	s := sortableIndices{
		indices: []primitives.ValidatorIndex{
			4,
			2,
			5,
			3,
			1,
			0,
		},
		state: st,
	}

	sort.Sort(s)

	want := []primitives.ValidatorIndex{
		0,
		5,
		4,
		2, // Validators with the same ActivationEligibilityEpoch are sorted by index, ascending.
		3,
		1,
	}

	if !cmp.Equal(s.indices, want) {
		t.Errorf("Failed to sort indices correctly, wanted %v, got %v", want, s.indices)
	}
}
