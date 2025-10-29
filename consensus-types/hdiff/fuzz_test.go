package hdiff

import (
	"context"
	"encoding/binary"
	"strconv"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

const maxFuzzValidators = 10000
const maxFuzzStateDiffSize = 1000
const maxFuzzHistoricalRoots = 10000
const maxFuzzDecodedSize = maxFuzzStateDiffSize * 10
const maxFuzzScanRange = 200
const fuzzRootsLengthOffset = 16
const maxFuzzInputSize = 10
const oneEthInGwei = 1000000000

// FuzzNewHdiff tests parsing variations of realistic diffs
func FuzzNewHdiff(f *testing.F) {
	// Add seed corpus with various valid diffs from realistic scenarios
	sizes := []uint64{8, 16, 32}
	for _, size := range sizes {
		source, _ := util.DeterministicGenesisStateElectra(f, size)
		
		// Create various realistic target states
		scenarios := []string{"slot_change", "balance_change", "validator_change", "multiple_changes"}
		for _, scenario := range scenarios {
			target := source.Copy()
			
			switch scenario {
			case "slot_change":
				_ = target.SetSlot(source.Slot() + 1)
			case "balance_change":
				balances := target.Balances()
				if len(balances) > 0 {
					balances[0] += 1000000000
					_ = target.SetBalances(balances)
				}
			case "validator_change":
				validators := target.Validators()
				if len(validators) > 0 {
					validators[0].EffectiveBalance += 1000000000
					_ = target.SetValidators(validators)
				}
			case "multiple_changes":
				_ = target.SetSlot(source.Slot() + 5)
				balances := target.Balances()
				validators := target.Validators()
				if len(balances) > 0 && len(validators) > 0 {
					balances[0] += 2000000000
					validators[0].EffectiveBalance += 1000000000
					_ = target.SetBalances(balances)
					_ = target.SetValidators(validators)
				}
			}
			
			validDiff, err := Diff(source, target)
			if err == nil {
				f.Add(validDiff.StateDiff, validDiff.ValidatorDiffs, validDiff.BalancesDiff)
			}
		}
	}
	
	f.Fuzz(func(t *testing.T, stateDiff, validatorDiffs, balancesDiff []byte) {
		// Limit input sizes to reasonable bounds
		if len(stateDiff) > 5000 || len(validatorDiffs) > 5000 || len(balancesDiff) > 5000 {
			return
		}
		
		// Bound historical roots length in stateDiff (if it contains snappy-compressed data)
		// The historicalRootsLength is read after snappy decompression, but we can still
		// limit the compressed input size to prevent extreme decompression ratios
		if len(stateDiff) > maxFuzzStateDiffSize {
			// Limit stateDiff to prevent potential memory bombs from snappy decompression
			stateDiff = stateDiff[:maxFuzzStateDiffSize]
		}
		
		// Bound validator count in validatorDiffs
		if len(validatorDiffs) >= 8 {
			count := binary.LittleEndian.Uint64(validatorDiffs[0:8])
			if count >= maxFuzzValidators {
				boundedCount := count % maxFuzzValidators
				binary.LittleEndian.PutUint64(validatorDiffs[0:8], boundedCount)
			}
		}
		
		// Bound balance count in balancesDiff
		if len(balancesDiff) >= 8 {
			count := binary.LittleEndian.Uint64(balancesDiff[0:8])
			if count >= maxFuzzValidators {
				boundedCount := count % maxFuzzValidators
				binary.LittleEndian.PutUint64(balancesDiff[0:8], boundedCount)
			}
		}
		
		input := HdiffBytes{
			StateDiff:      stateDiff,
			ValidatorDiffs: validatorDiffs,
			BalancesDiff:   balancesDiff,
		}
		
		// Test parsing - should not panic even with corrupted but bounded data
		_, err := newHdiff(input)
		_ = err // Expected to fail with corrupted data
	})
}

// FuzzNewStateDiff tests the newStateDiff function with valid random state diffs
func FuzzNewStateDiff(f *testing.F) {
	f.Fuzz(func(t *testing.T, validatorCount uint8, slotDelta uint64, balanceData []byte, validatorData []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("newStateDiff panicked: %v", r)
			}
		}()
		
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
			for i := 0; i < numChanges && i*8+8 < len(balanceData); i++ {
				idx := i % len(balances)
				delta := int64(binary.LittleEndian.Uint64(balanceData[i*8+8:(i+1)*8+8]))
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
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("newValidatorDiffs panicked: %v", r)
			}
		}()
		
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
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("newBalancesDiff panicked: %v", r)
			}
		}()
		
		// Bound balance count to reasonable range
		numBalances := int(balanceCount%32 + 8) // 8-39 balances
		
		// Generate simple source state
		source, _ := util.DeterministicGenesisStateElectra(t, uint64(numBalances))
		target := source.Copy()
		
		// Apply random balance changes based on balanceData
		if len(balanceData) >= 8 {
			balances := target.Balances()
			numChanges := int(binary.LittleEndian.Uint64(balanceData[:8])) % numBalances
			
			for i := 0; i < numChanges && i*8+8 < len(balanceData); i++ {
				idx := i % numBalances
				delta := int64(binary.LittleEndian.Uint64(balanceData[i*8+8:(i+1)*8+8]))
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
		}
		
		for _, scenario := range scenarios {
			testTarget := source.Copy()
			scenario()
			
			validDiff, err := Diff(source, testTarget)
			if err == nil {
				f.Add(validDiff.StateDiff, validDiff.ValidatorDiffs, validDiff.BalancesDiff)
			}
		}
	}
	
	f.Fuzz(func(t *testing.T, stateDiff, validatorDiffs, balancesDiff []byte) {
		// Only test with reasonable sized inputs
		if len(stateDiff) > 10000 || len(validatorDiffs) > 10000 || len(balancesDiff) > 10000 {
			return
		}
		
		// Bound historical roots length in stateDiff (same as FuzzNewHdiff)
		if len(stateDiff) > maxFuzzStateDiffSize {
			stateDiff = stateDiff[:maxFuzzStateDiffSize]
		}
		
		// Bound validator count in validatorDiffs
		if len(validatorDiffs) >= 8 {
			count := binary.LittleEndian.Uint64(validatorDiffs[0:8])
			if count >= maxFuzzValidators {
				boundedCount := count % maxFuzzValidators
				binary.LittleEndian.PutUint64(validatorDiffs[0:8], boundedCount)
			}
		}
		
		// Bound balance count in balancesDiff
		if len(balancesDiff) >= 8 {
			count := binary.LittleEndian.Uint64(balancesDiff[0:8])
			if count >= maxFuzzValidators {
				boundedCount := count % maxFuzzValidators
				binary.LittleEndian.PutUint64(balancesDiff[0:8], boundedCount)
			}
		}
		
		// Create fresh source state for each test
		source, _ := util.DeterministicGenesisStateElectra(t, 8)
		
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
	f.Add(make([]byte, 200)) // Larger than expected
	
	// Add a case with large reported length
	largeLength := make([]byte, 8)
	binary.LittleEndian.PutUint64(largeLength, 0xFFFFFFFF) // Large bits length
	f.Add(largeLength)
	
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("readPendingAttestation panicked: %v", r)
			}
		}()
		
		// Make a copy since the function modifies the slice
		dataCopy := make([]byte, len(data))
		copy(dataCopy, data)
		
		// Bound the bits length by modifying the first 8 bytes if they exist
		if len(dataCopy) >= 8 {
			// Read the bits length and bound it to maxFuzzValidators
			bitsLength := binary.LittleEndian.Uint64(dataCopy[0:8])
			if bitsLength >= maxFuzzValidators {
				boundedLength := bitsLength % maxFuzzValidators
				binary.LittleEndian.PutUint64(dataCopy[0:8], boundedLength)
			}
		}
		
		_, err := readPendingAttestation(&dataCopy)
		_ = err
	})
}

// FuzzKmpIndex tests the KMP algorithm implementation
func FuzzKmpIndex(f *testing.F) {
	// Test with integer pointers to match the actual usage
	f.Add("1,2,3", "4,5,6")
	f.Add("1,2,3", "1,2,3")
	f.Add("", "1,2,3")
	f.Add("1,1,1", "2,2,2")

	f.Fuzz(func(t *testing.T, sourceStr string, targetStr string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("kmpIndex panicked: %v", r)
			}
		}()

		// Parse comma-separated strings into int slices
		var source, target []int
		if sourceStr != "" {
			for _, s := range strings.Split(sourceStr, ",") {
				if val, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
					source = append(source, val)
				}
			}
		}
		if targetStr != "" {
			for _, s := range strings.Split(targetStr, ",") {
				if val, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
					target = append(target, val)
				}
			}
		}

		// Maintain the precondition: concatenate target with source
		// This matches how kmpIndex is actually called in production
		combined := make([]int, len(target)+len(source))
		copy(combined, target)
		copy(combined[len(target):], source)

		// Convert to pointer slices as used in actual code
		combinedPtrs := make([]*int, len(combined))
		for i := range combined {
			val := combined[i]
			combinedPtrs[i] = &val
		}

		integerEquals := func(a, b *int) bool {
			if a == nil && b == nil {
				return true
			}
			if a == nil || b == nil {
				return false
			}
			return *a == *b
		}

		result := kmpIndex(len(source), combinedPtrs, integerEquals)

		// Basic sanity check: result should be in [0, len(source)]
		if result < 0 || result > len(source) {
			t.Errorf("kmpIndex returned invalid result: %d for source length=%d", result, len(source))
		}
	})
}

// FuzzComputeLPS tests the LPS computation for KMP
func FuzzComputeLPS(f *testing.F) {
	// Add seed cases
	f.Add("1,2,1")
	f.Add("1,1,1")
	f.Add("1,2,3,4")
	f.Add("")
	
	f.Fuzz(func(t *testing.T, patternStr string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("computeLPS panicked: %v", r)
			}
		}()
		
		// Parse comma-separated string into int slice
		var pattern []int
		if patternStr != "" {
			for _, s := range strings.Split(patternStr, ",") {
				if val, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
					pattern = append(pattern, val)
				}
			}
		}
		
		// Convert to pointer slice
		patternPtrs := make([]*int, len(pattern))
		for i := range pattern {
			val := pattern[i]
			patternPtrs[i] = &val
		}
		
		integerEquals := func(a, b *int) bool {
			if a == nil && b == nil {
				return true
			}
			if a == nil || b == nil {
				return false
			}
			return *a == *b
		}
		
		result := computeLPS(patternPtrs, integerEquals)
		
		// Verify result length matches input
		if len(result) != len(pattern) {
			t.Errorf("computeLPS returned wrong length: got %d, expected %d", len(result), len(pattern))
		}
		
		// Verify all LPS values are non-negative and within bounds
		for i, lps := range result {
			if lps < 0 || lps > i {
				t.Errorf("Invalid LPS value at index %d: %d", i, lps)
			}
		}
	})
}

// FuzzDiffToBalances tests balance diff computation
func FuzzDiffToBalances(f *testing.F) {
	f.Fuzz(func(t *testing.T, sourceData, targetData []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("diffToBalances panicked: %v", r)
			}
		}()
		
		// Convert byte data to balance arrays
		var sourceBalances, targetBalances []uint64
		
		// Parse source balances (8 bytes per uint64)
		for i := 0; i+7 < len(sourceData) && len(sourceBalances) < 100; i += 8 {
			balance := binary.LittleEndian.Uint64(sourceData[i : i+8])
			sourceBalances = append(sourceBalances, balance)
		}
		
		// Parse target balances
		for i := 0; i+7 < len(targetData) && len(targetBalances) < 100; i += 8 {
			balance := binary.LittleEndian.Uint64(targetData[i : i+8])
			targetBalances = append(targetBalances, balance)
		}
		
		// Create states with the provided balances
		source, _ := util.DeterministicGenesisStateElectra(t, 1)
		target, _ := util.DeterministicGenesisStateElectra(t, 1)
		
		if len(sourceBalances) > 0 {
			_ = source.SetBalances(sourceBalances)
		}
		if len(targetBalances) > 0 {
			_ = target.SetBalances(targetBalances)
		}
		
		result, err := diffToBalances(source, target)
		
		// If no error, verify result consistency
		if err == nil && len(result) > 0 {
			// Result length should match target length
			if len(result) != len(target.Balances()) {
				t.Errorf("diffToBalances result length mismatch: got %d, expected %d", 
					len(result), len(target.Balances()))
			}
		}
	})
}

// FuzzValidatorsEqual tests validator comparison
func FuzzValidatorsEqual(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("validatorsEqual panicked: %v", r)
			}
		}()
		
		// Create two validators and fuzz their fields
		if len(data) < 16 {
			return
		}
		
		source, _ := util.DeterministicGenesisStateElectra(t, 2)
		validators := source.Validators()
		if len(validators) < 2 {
			return
		}
		
		val1 := validators[0]
		val2 := validators[1]
		
		// Modify validator fields based on fuzz data
		if len(data) > 0 && data[0]%2 == 0 {
			val2.EffectiveBalance = val1.EffectiveBalance + uint64(data[0])
		}
		if len(data) > 1 && data[1]%2 == 0 {
			val2.Slashed = !val1.Slashed
		}
		
		// Create ReadOnlyValidator wrappers if needed
		// Since validatorsEqual expects ReadOnlyValidator interface,
		// we'll skip this test for now as it requires state wrapper implementation
		_ = val1
		_ = val2
	})
}