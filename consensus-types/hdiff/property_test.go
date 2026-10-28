package hdiff

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// FuzzPropertyRoundTrip checks that applying Diff(source, target) to source reproduces target exactly.
func FuzzPropertyRoundTrip(f *testing.F) {
	// Slot, one balance increase and one effective balance change.
	f.Add(uint64(3), []byte("\x00\xca\x9a\x3b\x00\x00\x00\x00"), []byte{0})
	// Balance decrease and every validator change type.
	f.Add(uint64(0), []byte("\x00\x36\x65\xc4\xff\xff\xff\xff"), []byte{0, 1, 2, 3})
	// Slot near the uint64 limit and a balance that wraps past 2^63.
	f.Add(uint64(math.MaxUint64-1), []byte("\x00\x00\x00\x00\x00\x00\x00\x80"), []byte{})

	const validatorCount = 32
	base, _ := util.DeterministicGenesisStateElectra(f, validatorCount)
	f.Fuzz(func(t *testing.T, slotDelta uint64, balanceData, validatorData []byte) {
		target := base.Copy()
		require.NoError(t, target.SetSlot(base.Slot()+primitives.Slot(slotDelta)))

		// 8 bytes per balance delta, applied in order, wrapping.
		balances := target.Balances()
		for i := 0; i+8 <= len(balanceData) && i/8 < validatorCount; i += 8 {
			balances[i/8] += binary.LittleEndian.Uint64(balanceData[i : i+8])
		}
		require.NoError(t, target.SetBalances(balances))

		// One byte per validator; the value picks the field to change.
		validators := target.Validators()
		for i, b := range validatorData[:min(len(validatorData), validatorCount)] {
			switch b % 4 {
			case 0:
				validators[i].EffectiveBalance += oneEthInGwei
			case 1:
				validators[i].Slashed = !validators[i].Slashed
			case 2:
				validators[i].ActivationEpoch++
			case 3:
				validators[i].ExitEpoch++
			}
		}
		require.NoError(t, target.SetValidators(validators))

		diff, err := Diff(base, target)
		require.NoError(t, err)
		// ApplyDiff mutates its source.
		result, err := ApplyDiff(t.Context(), base.Copy(), diff)
		require.NoError(t, err)

		want, err := target.HashTreeRoot(t.Context())
		require.NoError(t, err)
		got, err := result.HashTreeRoot(t.Context())
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}

// PropertyTestDiffSize verifies that diffs are smaller than full states for typical cases
func FuzzPropertyDiffEfficiency(f *testing.F) {
	base, _ := util.DeterministicGenesisStateElectra(f, 64)
	f.Fuzz(func(t *testing.T, slotDelta uint64, numChanges uint8) {
		if slotDelta > 100 {
			slotDelta = slotDelta % 100
		}
		if numChanges > 10 {
			numChanges = numChanges % 10
		}

		// Create states with small differences
		source := base.Copy()
		target := source.Copy()

		_ = target.SetSlot(source.Slot() + primitives.Slot(slotDelta))

		// Make a few small changes
		if numChanges > 0 {
			validators := target.Validators()
			for i := uint8(0); i < numChanges && int(i) < len(validators); i++ {
				validators[i].EffectiveBalance += 1000
			}
			_ = target.SetValidators(validators)
		}

		// Create diff
		diff, err := Diff(source, target)
		if err != nil {
			return
		}

		// For small changes, diff should be much smaller than full state
		sourceSSZ, err := source.MarshalSSZ()
		if err != nil {
			return
		}

		diffSize := len(diff.StateDiff) + len(diff.ValidatorDiffs) + len(diff.BalancesDiff)

		// Diff should be smaller than full state for small changes
		if numChanges <= 5 && slotDelta <= 10 {
			require.Equal(t, true, diffSize < len(sourceSSZ)/2,
				"Diff size %d should be less than half of state size %d", diffSize, len(sourceSSZ))
		}
	})
}
