package util

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func SlotAtEpoch(t *testing.T, e primitives.Epoch) primitives.Slot {
	s, err := slots.EpochStart(e)
	require.NoError(t, err)
	return s
}
