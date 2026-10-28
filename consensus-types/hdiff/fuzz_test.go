package hdiff

import (
	"context"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

const (
	oneEthInGwei = 1000000000
	// Caps the StateDiff decode buffer at 8 MiB (= 256KiB * 32) via snappyDecode.
	maxFuzzStateDiffSize = 256 << 10
)

// snappyBombSeed is a 15-byte StateDiff whose snappy header declares 2.37 GiB of decoded data.
// https://github.com/OffchainLabs/prysm/actions/runs/32725115585/job/97425742241
// Kept as hex escapes: bytes 2-3 were once written as the literal character U+017D, which
// only happens to be \xc5\xbd while the file stays UTF-8.
var snappyBombSeed = [][]byte{
	[]byte("\xff\xc5\xbd\xbd\t\t\t\t\t\xbd\xb6\xaf\xbd\xbd0"),
	[]byte("\xff"),
	[]byte("p"),
}

// FuzzNewStateDiff tests the newStateDiff function with valid random state diffs
func FuzzNewStateDiff(f *testing.F) {
	f.Fuzz(func(t *testing.T, validatorCount uint8, slotDelta uint64, balanceData []byte, validatorData []byte) {
		// Bound validator count to reasonable range
		validators := uint64(validatorCount%32 + 8) // 8-39 validators
		if slotDelta > 100 {
			slotDelta = slotDelta % 100
		}

		// Generate random source state
		source, _ := util.DeterministicGenesisStateElectra(t, validators)
		target := source.Copy()

		// Apply random slot change
		_ = target.SetSlot(source.Slot() + primitives.Slot(slotDelta))

		// Apply random balance changes
		if len(balanceData) >= 8 {
			balances := target.Balances()
			numChanges := int(binary.LittleEndian.Uint64(balanceData[:8])) % len(balances)
			for i := 0; i < numChanges && (i+2)*8 <= len(balanceData); i++ {
				idx := i % len(balances)
				delta := int64(binary.LittleEndian.Uint64(balanceData[i*8+8 : (i+1)*8+8]))
				// Keep delta reasonable
				delta = delta % oneEthInGwei // Max 1 ETH change

				if delta < 0 && uint64(-delta) > balances[idx] {
					balances[idx] = 0
				} else if delta < 0 {
					balances[idx] -= uint64(-delta)
				} else {
					balances[idx] += uint64(delta)
				}
			}
			_ = target.SetBalances(balances)
		}

		// Apply random validator changes
		if len(validatorData) > 0 {
			validators := target.Validators()
			numChanges := int(validatorData[0]) % len(validators)
			for i := 0; i < numChanges && i < len(validatorData)-1; i++ {
				idx := i % len(validators)
				if validatorData[i+1]%2 == 0 {
					validators[idx].EffectiveBalance += oneEthInGwei // 1 ETH
				}
			}
			_ = target.SetValidators(validators)
		}

		// Create diff between source and target
		diff, err := Diff(source, target)
		if err != nil {
			return // Skip if diff creation fails
		}

		// Test newStateDiff with the valid serialized diff from StateDiff field
		reconstructed, err := newStateDiff(diff.StateDiff)
		if err != nil {
			t.Errorf("newStateDiff failed on valid diff: %v", err)
			return
		}

		// Basic validation that reconstruction worked
		if reconstructed == nil {
			t.Error("newStateDiff returned nil without error")
		}
	})
}

// FuzzNewValidatorDiffs tests validator diff deserialization with valid diffs
func FuzzNewValidatorDiffs(f *testing.F) {
	f.Fuzz(func(t *testing.T, validatorCount uint8, changeData []byte) {
		// Bound validator count to reasonable range
		validators := uint64(validatorCount%16 + 4) // 4-19 validators

		// Generate random source state
		source, _ := util.DeterministicGenesisStateElectra(t, validators)
		target := source.Copy()

		// Apply random validator changes based on changeData
		if len(changeData) > 0 {
			vals := target.Validators()
			numChanges := int(changeData[0]) % len(vals)

			for i := 0; i < numChanges && i < len(changeData)-1; i++ {
				idx := i % len(vals)
				changeType := changeData[i+1] % 4

				switch changeType {
				case 0: // Change effective balance
					vals[idx].EffectiveBalance += oneEthInGwei
				case 1: // Toggle slashed status
					vals[idx].Slashed = !vals[idx].Slashed
				case 2: // Change activation epoch
					vals[idx].ActivationEpoch++
				case 3: // Change exit epoch
					vals[idx].ExitEpoch++
				}
			}
			_ = target.SetValidators(vals)
		}

		// Create diff between source and target
		diff, err := Diff(source, target)
		if err != nil {
			return // Skip if diff creation fails
		}

		// Test newValidatorDiffs with the valid serialized diff
		reconstructed, err := newValidatorDiffs(diff.ValidatorDiffs)
		if err != nil {
			t.Errorf("newValidatorDiffs failed on valid diff: %v", err)
			return
		}

		// Basic validation that reconstruction worked
		if reconstructed == nil {
			t.Error("newValidatorDiffs returned nil without error")
		}
	})
}

// FuzzNewBalancesDiff tests balance diff deserialization with valid diffs
func FuzzNewBalancesDiff(f *testing.F) {
	f.Fuzz(func(t *testing.T, balanceCount uint8, balanceData []byte) {
		// Bound balance count to reasonable range
		numBalances := int(balanceCount%32 + 8) // 8-39 balances

		// Generate simple source state
		source, _ := util.DeterministicGenesisStateElectra(t, uint64(numBalances))
		target := source.Copy()

		// Apply random balance changes based on balanceData
		if len(balanceData) >= 8 {
			balances := target.Balances()
			numChanges := int(binary.LittleEndian.Uint64(balanceData[:8])) % numBalances

			for i := 0; i < numChanges && (i+2)*8 <= len(balanceData); i++ {
				idx := i % numBalances
				delta := int64(binary.LittleEndian.Uint64(balanceData[i*8+8 : (i+1)*8+8]))
				// Keep delta reasonable
				delta = delta % oneEthInGwei // Max 1 ETH change

				if delta < 0 && uint64(-delta) > balances[idx] {
					balances[idx] = 0
				} else if delta < 0 {
					balances[idx] -= uint64(-delta)
				} else {
					balances[idx] += uint64(delta)
				}
			}
			_ = target.SetBalances(balances)
		}

		// Create diff between source and target to get BalancesDiff
		diff, err := Diff(source, target)
		if err != nil {
			return // Skip if diff creation fails
		}

		// Test newBalancesDiff with the valid serialized diff
		reconstructed, err := newBalancesDiff(diff.BalancesDiff)
		if err != nil {
			t.Errorf("newBalancesDiff failed on valid diff: %v", err)
			return
		}

		// Basic validation that reconstruction worked
		if reconstructed == nil {
			t.Error("newBalancesDiff returned nil without error")
		}
	})
}

// FuzzApplyDiff tests applying variations of valid diffs
func FuzzApplyDiff(f *testing.F) {
	// Test with realistic state variations, not random data
	ctx := context.Background()

	// Add seed corpus with various valid scenarios
	sizes := []uint64{8, 16, 32, 64}
	for _, size := range sizes {
		source, _ := util.DeterministicGenesisStateElectra(f, size)
		target := source.Copy()

		// Different types of realistic changes
		scenarios := []func(){
			func() { _ = target.SetSlot(source.Slot() + 1) }, // Slot change
			func() { // Balance change
				balances := target.Balances()
				if len(balances) > 0 {
					balances[0] += 1000000000 // 1 ETH
					_ = target.SetBalances(balances)
				}
			},
			func() { // Validator change
				validators := target.Validators()
				if len(validators) > 0 {
					validators[0].EffectiveBalance += 1000000000
					_ = target.SetValidators(validators)
				}
			},
			func() { // Multiple changes
				_ = target.SetSlot(source.Slot() + 5)
				balances := target.Balances()
				validators := target.Validators()
				if len(balances) > 0 && len(validators) > 0 {
					balances[0] += 2000000000
					validators[0].EffectiveBalance += 1000000000
					_ = target.SetBalances(balances)
					_ = target.SetValidators(validators)
				}
			},
		}

		for _, scenario := range scenarios {
			target = source.Copy()
			scenario()

			validDiff, err := Diff(source, target)
			if err == nil {
				f.Add(validDiff.StateDiff, validDiff.ValidatorDiffs, validDiff.BalancesDiff)
			}
		}
	}

	f.Add(snappyBombSeed[0], snappyBombSeed[1], snappyBombSeed[2])

	base, _ := util.DeterministicGenesisStateElectra(f, 8)
	f.Fuzz(func(t *testing.T, stateDiff, validatorDiffs, balancesDiff []byte) {
		// Only test with reasonable sized inputs
		if len(stateDiff) > maxFuzzStateDiffSize || len(validatorDiffs) > 10000 || len(balancesDiff) > 10000 {
			return
		}

		// ApplyDiff mutates source, so copy to keep base unchanged across execs
		source := base.Copy()

		diff := HdiffBytes{
			StateDiff:      stateDiff,
			ValidatorDiffs: validatorDiffs,
			BalancesDiff:   balancesDiff,
		}

		// Apply diff - errors are expected for fuzzed data
		_, err := ApplyDiff(ctx, source, diff)
		_ = err // Expected to fail with invalid data
	})
}

// FuzzReadPendingAttestation tests the pending attestation deserialization
func FuzzReadPendingAttestation(f *testing.F) {
	// Add edge cases - this function is particularly vulnerable
	f.Add([]byte{})
	f.Add([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}) // 8 bytes
	f.Add(make([]byte, 200))                                      // Larger than expected

	// Add a case with large reported length
	largeLength := make([]byte, 8)
	binary.LittleEndian.PutUint64(largeLength, 0xFFFFFFFF) // Large bits length
	f.Add(largeLength)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = readPendingAttestation(&data)
	})
}

// FuzzKmpIndex checks computeLPS and kmpIndex against naive definitions, on the layout the
// diff code builds: target, a nil separator, then source.
func FuzzKmpIndex(f *testing.F) {
	// Source tail [2 3] is the target head: index 1.
	f.Add([]byte{2, 3, 0}, []byte{1, 2, 3})
	// Repeated values make computeLPS fall back to a shorter prefix.
	f.Add([]byte{1, 1, 2}, []byte{1, 1, 1, 1, 2})

	equals := func(a, b *int) bool {
		if a == nil || b == nil {
			return a == b
		}
		return *a == *b
	}
	// One element per byte, values mod 4 so that overlaps are common, at most 64 elements.
	toPtrs := func(data []byte) []*int {
		data = data[:min(len(data), 64)]
		ptrs := make([]*int, len(data))
		for i, b := range data {
			v := int(b % 4)
			ptrs[i] = &v
		}
		return ptrs
	}

	f.Fuzz(func(t *testing.T, targetData, sourceData []byte) {
		target := toPtrs(targetData)
		source := toPtrs(sourceData)
		combined := append(append(append([]*int{}, target...), nil), source...)

		// lps[i] is the longest proper prefix of combined[:i+1] that is also its suffix.
		lps := computeLPS(combined, equals)
		require.Equal(t, len(combined), len(lps))
		for i := range combined {
			want := 0
			for l := i; l > 0; l-- {
				if slices.EqualFunc(combined[:l], combined[i+1-l:i+1], equals) {
					want = l
					break
				}
			}
			require.Equal(t, want, lps[i], "lps[%d]", i)
		}

		// Naive: The index is the smallest k such that source[k:] is a prefix of target.
		want := len(source)
		for k := range source {
			n := len(source) - k
			if n <= len(target) && slices.EqualFunc(source[k:], target[:n], equals) {
				want = k
				break
			}
		}
		require.Equal(t, want, kmpIndex(len(source), combined, equals))
	})
}

// FuzzDiffToBalances checks that each diff entry adds back to the target balance, modulo 2^64.
func FuzzDiffToBalances(f *testing.F) {
	source, _ := util.DeterministicGenesisStateElectra(f, 1)
	target := source.Copy()

	// Balance decreases: source [10], target [3].
	f.Add([]byte("\x0a\x00\x00\x00\x00\x00\x00\x00"), []byte("\x03\x00\x00\x00\x00\x00\x00\x00"))
	// Target appends a balance: source [1], target [1 2].
	f.Add([]byte("\x01\x00\x00\x00\x00\x00\x00\x00"), []byte("\x01\x00\x00\x00\x00\x00\x00\x00\x02\x00\x00\x00\x00\x00\x00\x00"))

	// 8 bytes per balance, at most 100 balances.
	toBalances := func(data []byte) []uint64 {
		balances := make([]uint64, 0, min(len(data)/8, 100))
		for i := 0; i+8 <= len(data) && len(balances) < 100; i += 8 {
			balances = append(balances, binary.LittleEndian.Uint64(data[i:i+8]))
		}
		return balances
	}

	f.Fuzz(func(t *testing.T, sourceData, targetData []byte) {
		sourceBalances := toBalances(sourceData)
		targetBalances := toBalances(targetData)
		require.NoError(t, source.SetBalances(sourceBalances))
		require.NoError(t, target.SetBalances(targetBalances))

		diff, err := diffToBalances(source, target)
		if len(targetBalances) < len(sourceBalances) {
			require.ErrorContains(t, "less than source", err)
			return
		}
		require.NoError(t, err)
		require.Equal(t, len(targetBalances), len(diff))
		for i, want := range targetBalances {
			var s uint64
			if i < len(sourceBalances) {
				s = sourceBalances[i]
			}
			require.Equal(t, want, s+uint64(diff[i]), "balance %d", i)
		}
	})
}
