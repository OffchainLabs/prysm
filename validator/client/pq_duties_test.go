package client

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestPQSignedPreferences(t *testing.T) {
	preferences := func(slot primitives.Slot, gasLimit uint64) *ethpb.ProposerPreferences {
		return &ethpb.ProposerPreferences{
			DependentRoot:  make([]byte, fieldparams.RootLength),
			ProposalSlot:   slot,
			ValidatorIndex: 1,
			FeeRecipient:   make([]byte, fieldparams.FeeRecipientLength),
			TargetGasLimit: gasLimit,
		}
	}

	t.Run("empty", func(t *testing.T) {
		var signed pqSignedPreferences
		_, ok := signed.get(preferences(10, 1))
		require.Equal(t, false, ok)
	})

	t.Run("same preferences", func(t *testing.T) {
		var signed pqSignedPreferences
		expected := &ethpb.SignedProposerPreferences{Message: preferences(10, 1), Signature: []byte{0x01}}
		signed.set(expected)

		actual, ok := signed.get(preferences(10, 1))
		require.Equal(t, true, ok)
		require.Equal(t, expected, actual)
	})

	t.Run("other preferences for the same slot", func(t *testing.T) {
		var signed pqSignedPreferences
		signed.set(&ethpb.SignedProposerPreferences{Message: preferences(10, 1), Signature: []byte{0x01}})

		_, ok := signed.get(preferences(10, 2))
		require.Equal(t, false, ok)
	})

	t.Run("forgets old slots", func(t *testing.T) {
		var signed pqSignedPreferences
		signed.set(&ethpb.SignedProposerPreferences{Message: preferences(10, 1), Signature: []byte{0x01}})
		signed.set(&ethpb.SignedProposerPreferences{Message: preferences(10+pqSignedPreferencesSlotsKept+1, 1), Signature: []byte{0x01}})

		_, ok := signed.get(preferences(10, 1))
		require.Equal(t, false, ok)
	})
}
