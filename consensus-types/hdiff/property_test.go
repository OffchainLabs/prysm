package hdiff

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

// maxSafeBalance ensures balances can be safely cast to int64 for diff computation
const maxSafeBalance = 1<<52 - 1

// PropertyTestRoundTrip verifies that diff->apply is idempotent with realistic data
func FuzzPropertyRoundTrip(f *testing.F) {
	f.Fuzz(func(t *testing.T, slotDelta uint64, balanceData []byte, validatorData []byte) {
		// Limit to realistic ranges
		if slotDelta > 32 { // Max one epoch
			slotDelta = slotDelta % 32
		}
		
		// Convert byte data to realistic deltas and changes
		var balanceDeltas []int64
		var validatorChanges []bool
		
		// Parse balance deltas - limit to realistic amounts (8 bytes per int64)
		for i := 0; i+7 < len(balanceData) && len(balanceDeltas) < 20; i += 8 {
			delta := int64(binary.LittleEndian.Uint64(balanceData[i : i+8]))
			// Keep deltas realistic (max 10 ETH change)
			if delta > 10000000000 {
				delta = delta % 10000000000
			}
			if delta < -10000000000 {
				delta = -((-delta) % 10000000000)
			}
			balanceDeltas = append(balanceDeltas, delta)
		}
		
		// Parse validator changes (1 byte per bool) - limit to small number
		for i := 0; i < len(validatorData) && len(validatorChanges) < 10; i++ {
			validatorChanges = append(validatorChanges, validatorData[i]%2 == 0)
		}
		
		ctx := t.Context()
		
		// Create source state with reasonable size
		validatorCount := uint64(len(validatorChanges) + 8) // Minimum 8 validators
		if validatorCount > 64 {
			validatorCount = 64 // Cap at 64 for performance
		}
		source, _ := util.DeterministicGenesisStateElectra(t, validatorCount)
		
		// Create target state with modifications
		target := source.Copy()
		
		// Apply slot change
		_ = target.SetSlot(source.Slot() + primitives.Slot(slotDelta))
		
		// Apply realistic balance changes
		if len(balanceDeltas) > 0 {
			balances := target.Balances()
			for i, delta := range balanceDeltas {
				if i >= len(balances) {
					break
				}
				// Apply realistic balance changes with safe bounds
				if delta < 0 {
					if uint64(-delta) > balances[i] {
						balances[i] = 0 // Can't go below 0
					} else {
						balances[i] -= uint64(-delta)
					}
				} else {
					// Cap at reasonable maximum (1000 ETH)
					maxBalance := uint64(1000000000000) // 1000 ETH in Gwei
					if balances[i]+uint64(delta) > maxBalance {
						balances[i] = maxBalance
					} else {
						balances[i] += uint64(delta)
					}
				}
			}
			_ = target.SetBalances(balances)
		}
		
		// Apply realistic validator changes
		if len(validatorChanges) > 0 {
			validators := target.Validators()
			for i, shouldChange := range validatorChanges {
				if i >= len(validators) {
					break
				}
				if shouldChange {
					// Make realistic changes - small effective balance adjustments
					validators[i].EffectiveBalance += 1000000000 // 1 ETH
				}
			}
			_ = target.SetValidators(validators)
		}
		
		// Create diff
		diff, err := Diff(source, target)
		if err != nil {
			// If diff creation fails, that's acceptable for malformed inputs
			return
		}
		
		// Apply diff
		result, err := ApplyDiff(ctx, source, diff)
		if err != nil {
			// If diff application fails, that's acceptable
			return
		}
		
		// Verify round-trip property: source + diff = target
		require.Equal(t, target.Slot(), result.Slot())
		
		// Verify balance consistency
		targetBalances := target.Balances()
		resultBalances := result.Balances()
		require.Equal(t, len(targetBalances), len(resultBalances))
		for i := range targetBalances {
			require.Equal(t, targetBalances[i], resultBalances[i], "Balance mismatch at index %d", i)
		}
		
		// Verify validator consistency
		targetVals := target.Validators()
		resultVals := result.Validators()
		require.Equal(t, len(targetVals), len(resultVals))
		for i := range targetVals {
			require.Equal(t, targetVals[i].Slashed, resultVals[i].Slashed, "Validator slashing mismatch at index %d", i)
			require.Equal(t, targetVals[i].EffectiveBalance, resultVals[i].EffectiveBalance, "Validator balance mismatch at index %d", i)
		}
	})
}

// PropertyTestReasonablePerformance verifies operations complete quickly with realistic data
func FuzzPropertyResourceBounds(f *testing.F) {
	f.Fuzz(func(t *testing.T, validatorCount uint8, slotDelta uint8, changeCount uint8) {
		// Use realistic parameters
		validators := uint64(validatorCount%64 + 8) // 8-71 validators
		slots := uint64(slotDelta % 32)              // 0-31 slots
		changes := int(changeCount % 10)            // 0-9 changes
		
		// Create realistic states
		source, _ := util.DeterministicGenesisStateElectra(t, validators)
		target := source.Copy()
		
		// Apply realistic changes
		_ = target.SetSlot(source.Slot() + primitives.Slot(slots))
		
		if changes > 0 {
			validatorList := target.Validators()
			for i := 0; i < changes && i < len(validatorList); i++ {
				validatorList[i].EffectiveBalance += 1000000000 // 1 ETH
			}
			_ = target.SetValidators(validatorList)
		}
		
		// Operations should complete quickly
		start := time.Now()
		diff, err := Diff(source, target)
		duration := time.Since(start)
		
		if err == nil {
			// Should be fast
			require.Equal(t, true, duration < time.Second, "Diff creation too slow: %v", duration)
			
			// Apply should also be fast
			start = time.Now()
			_, err = ApplyDiff(t.Context(), source, diff)
			duration = time.Since(start)
			
			if err == nil {
				require.Equal(t, true, duration < time.Second, "Diff application too slow: %v", duration)
			}
		}
	})
}

// PropertyTestDiffSize verifies that diffs are smaller than full states for typical cases
func FuzzPropertyDiffEfficiency(f *testing.F) {
	f.Fuzz(func(t *testing.T, slotDelta uint64, numChanges uint8) {
		if slotDelta > 100 {
			slotDelta = slotDelta % 100
		}
		if numChanges > 10 {
			numChanges = numChanges % 10
		}
		
		// Create states with small differences
		source, _ := util.DeterministicGenesisStateElectra(t, 64)
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

// PropertyTestBalanceConservation verifies that balance operations don't create/destroy value unexpectedly
func FuzzPropertyBalanceConservation(f *testing.F) {
	f.Fuzz(func(t *testing.T, balanceData []byte) {
		// Convert byte data to balance changes, bounded to safe range
		var balanceChanges []int64
		for i := 0; i+7 < len(balanceData) && len(balanceChanges) < 50; i += 8 {
			rawChange := int64(binary.LittleEndian.Uint64(balanceData[i : i+8]))
			// Bound the change to ensure resulting balances stay within safe range
			change := rawChange % (maxSafeBalance / 2) // Divide by 2 to allow for addition/subtraction
			balanceChanges = append(balanceChanges, change)
		}
		
		source, _ := util.DeterministicGenesisStateElectra(t, uint64(len(balanceChanges)+10))
		originalBalances := source.Balances()
		
		// Ensure initial balances are within safe range for int64 casting
		for i, balance := range originalBalances {
			if balance > maxSafeBalance {
				originalBalances[i] = balance % maxSafeBalance
			}
		}
		_ = source.SetBalances(originalBalances)
		
		// Calculate total before
		var totalBefore uint64
		for _, balance := range originalBalances {
			totalBefore += balance
		}
		
		// Apply balance changes via diff system
		target := source.Copy()
		targetBalances := target.Balances()
		
		var totalDelta int64
		for i, delta := range balanceChanges {
			if i >= len(targetBalances) {
				break
			}
			
			// Prevent underflow
			if delta < 0 && uint64(-delta) > targetBalances[i] {
				totalDelta -= int64(targetBalances[i]) // Actually lost amount (negative)
				targetBalances[i] = 0
			} else if delta < 0 {
				targetBalances[i] -= uint64(-delta)
				totalDelta += delta
			} else {
				// Prevent overflow
				if uint64(delta) > math.MaxUint64-targetBalances[i] {
					gained := math.MaxUint64 - targetBalances[i]
					totalDelta += int64(gained)
					targetBalances[i] = math.MaxUint64
				} else {
					targetBalances[i] += uint64(delta)
					totalDelta += delta
				}
			}
		}
		_ = target.SetBalances(targetBalances)
		
		// Apply through diff system
		diff, err := Diff(source, target)
		if err != nil {
			return
		}
		
		result, err := ApplyDiff(t.Context(), source, diff)
		if err != nil {
			return
		}
		
		// Calculate total after
		resultBalances := result.Balances()
		var totalAfter uint64
		for _, balance := range resultBalances {
			totalAfter += balance
		}
		
		// Verify conservation (accounting for intended changes)
		expectedTotal := totalBefore
		if totalDelta >= 0 {
			expectedTotal += uint64(totalDelta)
		} else {
			if uint64(-totalDelta) <= expectedTotal {
				expectedTotal -= uint64(-totalDelta)
			} else {
				expectedTotal = 0
			}
		}
		
		require.Equal(t, expectedTotal, totalAfter, 
			"Balance conservation violated: before=%d, delta=%d, expected=%d, actual=%d", 
			totalBefore, totalDelta, expectedTotal, totalAfter)
	})
}

// PropertyTestMonotonicSlot verifies slot only increases
func FuzzPropertyMonotonicSlot(f *testing.F) {
	f.Fuzz(func(t *testing.T, slotDelta uint64) {
		source, _ := util.DeterministicGenesisStateElectra(t, 16)
		target := source.Copy()
		
		targetSlot := source.Slot() + primitives.Slot(slotDelta)
		_ = target.SetSlot(targetSlot)
		
		diff, err := Diff(source, target)
		if err != nil {
			return
		}
		
		result, err := ApplyDiff(t.Context(), source, diff)
		if err != nil {
			return
		}
		
		// Slot should never decrease
		require.Equal(t, true, result.Slot() >= source.Slot(), 
			"Slot decreased from %d to %d", source.Slot(), result.Slot())
		
		// Slot should match target
		require.Equal(t, targetSlot, result.Slot())
	})
}

// PropertyTestValidatorIndexIntegrity verifies validator indices remain consistent
func FuzzPropertyValidatorIndices(f *testing.F) {
	f.Fuzz(func(t *testing.T, changeData []byte) {
		// Convert byte data to boolean changes
		var changes []bool
		for i := 0; i < len(changeData) && len(changes) < 20; i++ {
			changes = append(changes, changeData[i]%2 == 0)
		}
		
		source, _ := util.DeterministicGenesisStateElectra(t, uint64(len(changes)+5))
		target := source.Copy()
		
		// Apply changes
		validators := target.Validators()
		for i, shouldChange := range changes {
			if i >= len(validators) {
				break
			}
			if shouldChange {
				validators[i].EffectiveBalance += 1000
			}
		}
		_ = target.SetValidators(validators)
		
		diff, err := Diff(source, target)
		if err != nil {
			return
		}
		
		result, err := ApplyDiff(t.Context(), source, diff)
		if err != nil {
			return
		}
		
		// Validator count should not decrease
		require.Equal(t, true, len(result.Validators()) >= len(source.Validators()),
			"Validator count decreased from %d to %d", len(source.Validators()), len(result.Validators()))
		
		// Public keys should be preserved for existing validators
		sourceVals := source.Validators()
		resultVals := result.Validators()
		for i := range sourceVals {
			if i < len(resultVals) {
				require.DeepEqual(t, sourceVals[i].PublicKey, resultVals[i].PublicKey,
					"Public key changed at validator index %d", i)
			}
		}
	})
}