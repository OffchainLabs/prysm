package helpers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestPayloadCommitteeAvailable(t *testing.T) {
	spe := uint64(params.BeaconConfig().SlotsPerEpoch)
	lookahead := uint64(params.BeaconConfig().MinSeedLookahead)
	// Anchor the head a few epochs in so cases can go backwards without underflowing.
	const stateEpoch = uint64(4)
	stateSlot := primitives.Slot(stateEpoch * spe)
	at := func(epoch uint64) primitives.Slot { return primitives.Slot(epoch * spe) }

	tests := []struct {
		name string
		slot primitives.Slot
		want bool
	}{
		{"same epoch, first slot", at(stateEpoch), true},
		{"same epoch, last slot", stateSlot + primitives.Slot(spe-1), true},
		{"one epoch behind (window floor)", at(stateEpoch - 1), true},
		{"two epochs behind (out of window)", at(stateEpoch - 2), false},
		{"lookahead boundary (window ceiling)", at(stateEpoch + lookahead), true},
		{"beyond lookahead (out of window)", at(stateEpoch + lookahead + 1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, helpers.PayloadCommitteeAvailable(stateSlot, tt.slot))
		})
	}
}
