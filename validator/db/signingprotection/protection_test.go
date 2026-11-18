package signingprotection

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSlotMessageString(t *testing.T) {
	t.Run("known messages", func(t *testing.T) {
		require.Equal(t, "block", Block.String())
		require.Equal(t, "aggregate and proof", AggregateAndProof.String())
		require.Equal(t, "payload attestation", PayloadAttestation.String())
		require.Equal(t, "proposer preferences", ProposerPreferences.String())
		require.Equal(t, "execution payload envelope", ExecutionPayloadEnvelope.String())
	})

	t.Run("unknown message", func(t *testing.T) {
		require.Equal(t, "unknown slot message 5", SlotMessage(5).String())
	})
}

func TestCheckAndSaveAttestation(t *testing.T) {
	pubkey := pubkeyFromByte(1)

	type attestation struct {
		source, target primitives.Epoch
		refused        bool
	}

	tests := []struct {
		name         string
		attestations []attestation
	}{
		{
			name:         "first attestation",
			attestations: []attestation{{source: 0, target: 1}},
		},
		{
			name:         "increasing source and target",
			attestations: []attestation{{source: 1, target: 2}, {source: 2, target: 3}},
		},
		{
			name:         "same source, higher target",
			attestations: []attestation{{source: 1, target: 2}, {source: 1, target: 5}},
		},
		{
			name:         "same target (double vote, same leaf)",
			attestations: []attestation{{source: 1, target: 2}, {source: 1, target: 2, refused: true}},
		},
		{
			name:         "same target, different source",
			attestations: []attestation{{source: 1, target: 3}, {source: 2, target: 3, refused: true}},
		},
		{
			name:         "lower target",
			attestations: []attestation{{source: 1, target: 3}, {source: 1, target: 2, refused: true}},
		},
		{
			name:         "surrounding vote",
			attestations: []attestation{{source: 2, target: 3}, {source: 1, target: 4, refused: true}},
		},
		{
			name:         "source higher than target",
			attestations: []attestation{{source: 3, target: 2, refused: true}},
		},
		{
			name:         "a refusal does not change the watermarks",
			attestations: []attestation{{source: 2, target: 3}, {source: 1, target: 10, refused: true}, {source: 2, target: 4}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
			require.NoError(t, err)

			var last *attestation
			for _, att := range tt.attestations {
				err := store.CheckAndSaveAttestation(pubkey, att.source, att.target)
				if att.refused {
					require.ErrorIs(t, err, ErrSigningRefused)
					continue
				}

				require.NoError(t, err)
				last = &att
			}

			protection, err := store.SigningProtection(pubkey)
			require.NoError(t, err)

			if last == nil {
				require.IsNil(t, protection.LastSignedAttestationSourceEpoch)
				require.IsNil(t, protection.LastSignedAttestationTargetEpoch)
				return
			}

			require.DeepEqual(t, new(uint64(last.source)), protection.LastSignedAttestationSourceEpoch)
			require.DeepEqual(t, new(uint64(last.target)), protection.LastSignedAttestationTargetEpoch)
		})
	}
}

func TestCheckAndSaveSlotMessage(t *testing.T) {
	pubkey := pubkeyFromByte(1)
	messages := []SlotMessage{Block, AggregateAndProof, PayloadAttestation, ProposerPreferences, ExecutionPayloadEnvelope}

	t.Run("increasing slots", func(t *testing.T) {
		for _, message := range messages {
			store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
			require.NoError(t, err)

			require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, message, 0))
			require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, message, 1))
			require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, message, 10))
		}
	})

	t.Run("same slot", func(t *testing.T) {
		for _, message := range messages {
			store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
			require.NoError(t, err)

			require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, message, 5))
			require.ErrorIs(t, store.CheckAndSaveSlotMessage(pubkey, message, 5), ErrSigningRefused)
		}
	})

	t.Run("lower slot", func(t *testing.T) {
		for _, message := range messages {
			store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
			require.NoError(t, err)

			require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, message, 5))
			require.ErrorIs(t, store.CheckAndSaveSlotMessage(pubkey, message, 4), ErrSigningRefused)

			protection, err := store.SigningProtection(pubkey)
			require.NoError(t, err)

			watermark, err := slotWatermark(protection, message)
			require.NoError(t, err)
			require.DeepEqual(t, new(uint64(5)), *watermark)
		}
	})

	t.Run("message types are independent", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		// Proposer preferences are signed ahead of time, for a slot of the next epoch.
		require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, ProposerPreferences, 40))

		for _, message := range messages {
			if message == ProposerPreferences {
				continue
			}

			require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, message, 10))
		}

		protection, err := store.SigningProtection(pubkey)
		require.NoError(t, err)

		expected := &ValidatorSigningProtection{
			LastSignedBlockSlot:                    new(uint64(10)),
			LastSignedAggregateAndProofSlot:        new(uint64(10)),
			LastSignedPayloadAttestationSlot:       new(uint64(10)),
			LastSignedProposerPreferencesSlot:      new(uint64(40)),
			LastSignedExecutionPayloadEnvelopeSlot: new(uint64(10)),
		}
		require.DeepEqual(t, expected, protection)
	})

	t.Run("public keys are independent", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		require.NoError(t, store.CheckAndSaveSlotMessage(pubkeyFromByte(1), Block, 5))
		require.NoError(t, store.CheckAndSaveSlotMessage(pubkeyFromByte(2), Block, 5))
	})

	t.Run("unknown message", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		require.ErrorIs(t, store.CheckAndSaveSlotMessage(pubkey, SlotMessage(5), 1), ErrSigningRefused)

		pubkeys, err := store.PublicKeys()
		require.NoError(t, err)
		require.Equal(t, 0, len(pubkeys))
	})
}

func TestNextReservedLeaf(t *testing.T) {
	pubkey := pubkeyFromByte(1)

	t.Run("never returns the preregistration leaf", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		for expected := uint64(1); expected <= 3; expected++ {
			leaf, err := store.NextReservedLeaf(pubkey)
			require.NoError(t, err)
			require.Equal(t, expected, leaf)
		}
	})

	t.Run("survives a restart", func(t *testing.T) {
		parent := t.TempDir()

		store, err := NewStore(parent, genesisValidatorsRoot)
		require.NoError(t, err)

		leaf, err := store.NextReservedLeaf(pubkey)
		require.NoError(t, err)
		require.Equal(t, uint64(1), leaf)

		reopened, err := NewStore(parent, genesisValidatorsRoot)
		require.NoError(t, err)

		leaf, err = reopened.NextReservedLeaf(pubkey)
		require.NoError(t, err)
		require.Equal(t, uint64(2), leaf)
	})

	t.Run("exhausted", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, store.Merge(pubkey, &ValidatorSigningProtection{LastSignedReservedLeaf: ReservedLeafCount - 2}))

		leaf, err := store.NextReservedLeaf(pubkey)
		require.NoError(t, err)
		require.Equal(t, ReservedLeafCount-1, leaf)

		_, err = store.NextReservedLeaf(pubkey)
		require.ErrorIs(t, err, ErrSigningRefused)

		protection, err := store.SigningProtection(pubkey)
		require.NoError(t, err)
		require.Equal(t, ReservedLeafCount-1, protection.LastSignedReservedLeaf)
	})
}

func TestMerge(t *testing.T) {
	pubkey := pubkeyFromByte(1)

	t.Run("nil", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, store.Merge(pubkey, nil))

		pubkeys, err := store.PublicKeys()
		require.NoError(t, err)
		require.Equal(t, 0, len(pubkeys))
	})

	t.Run("keeps the highest watermark of each field", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		require.NoError(t, store.Merge(pubkey, &ValidatorSigningProtection{
			LastSignedReservedLeaf:            5,
			LastSignedAttestationSourceEpoch:  new(uint64(10)),
			LastSignedAttestationTargetEpoch:  new(uint64(11)),
			LastSignedBlockSlot:               new(uint64(100)),
			LastSignedProposerPreferencesSlot: new(uint64(300)),
		}))

		require.NoError(t, store.Merge(pubkey, &ValidatorSigningProtection{
			LastSignedReservedLeaf:                 3,
			LastSignedAttestationSourceEpoch:       new(uint64(12)),
			LastSignedAttestationTargetEpoch:       new(uint64(9)),
			LastSignedBlockSlot:                    new(uint64(200)),
			LastSignedAggregateAndProofSlot:        new(uint64(50)),
			LastSignedPayloadAttestationSlot:       new(uint64(60)),
			LastSignedExecutionPayloadEnvelopeSlot: new(uint64(70)),
		}))

		protection, err := store.SigningProtection(pubkey)
		require.NoError(t, err)

		expected := &ValidatorSigningProtection{
			LastSignedReservedLeaf:                 5,
			LastSignedAttestationSourceEpoch:       new(uint64(12)),
			LastSignedAttestationTargetEpoch:       new(uint64(11)),
			LastSignedBlockSlot:                    new(uint64(200)),
			LastSignedAggregateAndProofSlot:        new(uint64(50)),
			LastSignedPayloadAttestationSlot:       new(uint64(60)),
			LastSignedProposerPreferencesSlot:      new(uint64(300)),
			LastSignedExecutionPayloadEnvelopeSlot: new(uint64(70)),
		}
		require.DeepEqual(t, expected, protection)
	})

	t.Run("does not alias the merged value", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		other := &ValidatorSigningProtection{LastSignedBlockSlot: new(uint64(10))}
		require.NoError(t, store.Merge(pubkey, other))
		require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, Block, 20))
		require.DeepEqual(t, new(uint64(10)), other.LastSignedBlockSlot)
	})
}
