package xmsssigner

import (
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
)

func TestAttestationLeafIndex(t *testing.T) {
	t.Run("first epoch", func(t *testing.T) {
		leafIndex, err := AttestationLeafIndex(0)
		require.NoError(t, err)
		require.Equal(t, uint32(1<<15), leafIndex)
	})

	t.Run("some epoch", func(t *testing.T) {
		leafIndex, err := AttestationLeafIndex(1201)
		require.NoError(t, err)
		require.Equal(t, uint32(1<<15+1201), leafIndex)
	})

	t.Run("last epoch", func(t *testing.T) {
		leafIndex, err := AttestationLeafIndex(1<<29 - 1)
		require.NoError(t, err)
		require.Equal(t, uint32(1<<15+1<<29-1), leafIndex)
	})

	t.Run("beyond the attestation leaves", func(t *testing.T) {
		_, err := AttestationLeafIndex(1 << 29)
		require.ErrorContains(t, "beyond the 536870912 attestation leaves", err)
	})
}

func TestSlotLeafIndex(t *testing.T) {
	const first = 1<<15 + 1<<29 + 1<<30

	t.Run("each message type", func(t *testing.T) {
		const slot = primitives.Slot(38450)

		testCases := []struct {
			message signingprotection.SlotMessage
			offset  uint32
		}{
			{message: signingprotection.Block, offset: 0},
			{message: signingprotection.AggregateAndProof, offset: 1},
			{message: signingprotection.PayloadAttestation, offset: 2},
			{message: signingprotection.ProposerPreferences, offset: 3},
			{message: signingprotection.ExecutionPayloadEnvelope, offset: 4},
		}

		for _, tc := range testCases {
			leafIndex, err := SlotLeafIndex(slot, tc.message)
			require.NoError(t, err)
			require.Equal(t, uint32(first+38450*8)+tc.offset, leafIndex)
		}
	})

	t.Run("last slot", func(t *testing.T) {
		const lastSlot = (math.MaxUint32 - first - 7) / 8

		leafIndex, err := SlotLeafIndex(lastSlot, signingprotection.ExecutionPayloadEnvelope)
		require.NoError(t, err)
		require.Equal(t, uint32(first+lastSlot*8+4), leafIndex)
	})

	t.Run("beyond the last slot", func(t *testing.T) {
		const lastSlot = (math.MaxUint32 - first - 7) / 8

		_, err := SlotLeafIndex(lastSlot+1, signingprotection.Block)
		require.ErrorContains(t, "beyond the last slot", err)
	})

	t.Run("offset not lower than the leaves per slot", func(t *testing.T) {
		_, err := SlotLeafIndex(0, signingprotection.SlotMessage(8))
		require.ErrorContains(t, "is not lower than 8", err)
	})
}
