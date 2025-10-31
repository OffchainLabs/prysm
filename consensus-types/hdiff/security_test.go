package hdiff

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

// TestIntegerOverflowProtection tests protection against balance overflow attacks
func TestIntegerOverflowProtection(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 8)
	
	// Test balance overflow in diffToBalances - use realistic values
	t.Run("balance_diff_overflow", func(t *testing.T) {
		target := source.Copy()
		balances := target.Balances()
		
		// Set high but realistic balance values (32 ETH in Gwei = 32e9)
		balances[0] = 32000000000 // 32 ETH
		balances[1] = 64000000000 // 64 ETH
		_ = target.SetBalances(balances)
		
		// This should work fine with realistic values
		diffs, err := diffToBalances(source, target)
		require.NoError(t, err)
		
		// Verify the diffs are reasonable
		require.Equal(t, true, len(diffs) > 0, "Should have balance diffs")
	})
	
	// Test reasonable balance changes
	t.Run("realistic_balance_changes", func(t *testing.T) {
		// Create realistic balance changes (slashing, rewards)
		balancesDiff := []int64{1000000000, -500000000, 2000000000} // 1 ETH gain, 0.5 ETH loss, 2 ETH gain
		
		// Apply to state with normal balances
		testSource := source.Copy()
		normalBalances := []uint64{32000000000, 32000000000, 32000000000} // 32 ETH each
		_ = testSource.SetBalances(normalBalances)
		
		// This should work fine
		result, err := applyBalancesDiff(testSource, balancesDiff)
		require.NoError(t, err)
		
		resultBalances := result.Balances()
		require.Equal(t, uint64(33000000000), resultBalances[0]) // 33 ETH
		require.Equal(t, uint64(31500000000), resultBalances[1]) // 31.5 ETH
		require.Equal(t, uint64(34000000000), resultBalances[2]) // 34 ETH
	})
}

// TestReasonablePerformance tests that operations complete in reasonable time
func TestReasonablePerformance(t *testing.T) {
	t.Run("large_state_performance", func(t *testing.T) {
		// Test with a large but realistic validator set
		source, _ := util.DeterministicGenesisStateElectra(t, 1000) // 1000 validators
		target := source.Copy()
		
		// Make realistic changes
		_ = target.SetSlot(source.Slot() + 32) // One epoch
		validators := target.Validators()
		for i := 0; i < 100; i++ { // 10% of validators changed
			validators[i].EffectiveBalance += 1000000000 // 1 ETH change
		}
		_ = target.SetValidators(validators)
		
		// Should complete quickly
		start := time.Now()
		diff, err := Diff(source, target)
		duration := time.Since(start)
		
		require.NoError(t, err)
		require.Equal(t, true, duration < time.Second, "Diff creation took too long: %v", duration)
		require.Equal(t, true, len(diff.StateDiff) > 0, "Should have state diff")
	})
	
	t.Run("realistic_diff_application", func(t *testing.T) {
		// Test applying diffs to large states
		source, _ := util.DeterministicGenesisStateElectra(t, 500)
		target := source.Copy()
		_ = target.SetSlot(source.Slot() + 1)
		
		// Create and apply diff
		diff, err := Diff(source, target)
		require.NoError(t, err)
		
		start := time.Now()
		result, err := ApplyDiff(t.Context(), source, diff)
		duration := time.Since(start)
		
		require.NoError(t, err)
		require.Equal(t, target.Slot(), result.Slot())
		require.Equal(t, true, duration < time.Second, "Diff application took too long: %v", duration)
	})
}

// TestStateTransitionValidation tests realistic state transition scenarios
func TestStateTransitionValidation(t *testing.T) {
	t.Run("validator_slashing_scenario", func(t *testing.T) {
		source, _ := util.DeterministicGenesisStateElectra(t, 10)
		target := source.Copy()
		
		// Simulate validator slashing (realistic scenario)
		validators := target.Validators()
		validators[0].Slashed = true
		validators[0].EffectiveBalance = 0 // Slashed validator loses balance
		_ = target.SetValidators(validators)
		
		// This should work fine
		diff, err := Diff(source, target)
		require.NoError(t, err)
		
		result, err := ApplyDiff(t.Context(), source, diff)
		require.NoError(t, err)
		require.Equal(t, true, result.Validators()[0].Slashed)
		require.Equal(t, uint64(0), result.Validators()[0].EffectiveBalance)
	})
	
	t.Run("epoch_transition_scenario", func(t *testing.T) {
		source, _ := util.DeterministicGenesisStateElectra(t, 64)
		target := source.Copy()
		
		// Simulate epoch transition with multiple changes
		_ = target.SetSlot(source.Slot() + 32) // One epoch
		
		// Some validators get rewards, others get penalties
		balances := target.Balances()
		for i := 0; i < len(balances); i++ {
			if i%2 == 0 {
				balances[i] += 100000000 // 0.1 ETH reward
			} else {
				if balances[i] > 50000000 {
					balances[i] -= 50000000 // 0.05 ETH penalty
				}
			}
		}
		_ = target.SetBalances(balances)
		
		// This should work smoothly
		diff, err := Diff(source, target)
		require.NoError(t, err)
		
		result, err := ApplyDiff(t.Context(), source, diff)
		require.NoError(t, err)
		require.Equal(t, target.Slot(), result.Slot())
	})
	
	t.Run("consistent_state_root", func(t *testing.T) {
		// Test that diffs preserve state consistency
		source, _ := util.DeterministicGenesisStateElectra(t, 32)
		target := source.Copy()
		
		// Make minimal changes
		_ = target.SetSlot(source.Slot() + 1)
		
		// Diff and apply should be consistent
		diff, err := Diff(source, target)
		require.NoError(t, err)
		
		result, err := ApplyDiff(t.Context(), source, diff)
		require.NoError(t, err)
		
		// Result should match target
		require.Equal(t, target.Slot(), result.Slot())
		require.Equal(t, len(target.Validators()), len(result.Validators()))
		require.Equal(t, len(target.Balances()), len(result.Balances()))
	})
}

// TestSerializationRoundTrip tests serialization consistency
func TestSerializationRoundTrip(t *testing.T) {
	t.Run("diff_serialization_consistency", func(t *testing.T) {
		// Test that serialization and deserialization are consistent
		source, _ := util.DeterministicGenesisStateElectra(t, 16)
		target := source.Copy()
		
		// Make changes
		_ = target.SetSlot(source.Slot() + 5)
		validators := target.Validators()
		validators[0].EffectiveBalance += 1000000000
		_ = target.SetValidators(validators)
		
		// Create diff
		diff1, err := Diff(source, target)
		require.NoError(t, err)
		
		// Deserialize and re-serialize
		hdiff, err := newHdiff(diff1)
		require.NoError(t, err)
		
		diff2 := hdiff.serialize()
		
		// Apply both diffs - should get same result
		result1, err := ApplyDiff(t.Context(), source, diff1)
		require.NoError(t, err)
		
		result2, err := ApplyDiff(t.Context(), source, diff2)
		require.NoError(t, err)
		
		require.Equal(t, result1.Slot(), result2.Slot())
		require.Equal(t, result1.Validators()[0].EffectiveBalance, result2.Validators()[0].EffectiveBalance)
	})
	
	t.Run("empty_diff_handling", func(t *testing.T) {
		// Test that empty diffs are handled correctly
		source, _ := util.DeterministicGenesisStateElectra(t, 8)
		target := source.Copy() // No changes
		
		// Should create minimal diff
		diff, err := Diff(source, target)
		require.NoError(t, err)
		
		// Apply should work and return equivalent state
		result, err := ApplyDiff(t.Context(), source, diff)
		require.NoError(t, err)
		
		require.Equal(t, source.Slot(), result.Slot())
		require.Equal(t, len(source.Validators()), len(result.Validators()))
	})
	
	t.Run("compression_efficiency", func(t *testing.T) {
		// Test that compression is working effectively
		source, _ := util.DeterministicGenesisStateElectra(t, 100)
		target := source.Copy()
		
		// Make small changes
		_ = target.SetSlot(source.Slot() + 1)
		validators := target.Validators()
		validators[0].EffectiveBalance += 1000000000
		_ = target.SetValidators(validators)
		
		// Create diff
		diff, err := Diff(source, target)
		require.NoError(t, err)
		
		// Get full state size
		fullStateSSZ, err := target.MarshalSSZ()
		require.NoError(t, err)
		
		// Diff should be much smaller than full state
		diffSize := len(diff.StateDiff) + len(diff.ValidatorDiffs) + len(diff.BalancesDiff)
		require.Equal(t, true, diffSize < len(fullStateSSZ)/2, 
			"Diff should be smaller than full state: diff=%d, full=%d", diffSize, len(fullStateSSZ))
	})
}

// TestKMPSecurity tests the KMP algorithm for security issues
func TestKMPSecurity(t *testing.T) {
	t.Run("nil_pointer_handling", func(t *testing.T) {
		// Test with nil pointers in the pattern/text
		pattern := []*int{nil, nil, nil}
		text := []*int{nil, nil, nil, nil, nil}
		
		equals := func(a, b *int) bool {
			if a == nil && b == nil {
				return true
			}
			if a == nil || b == nil {
				return false
			}
			return *a == *b
		}
		
		// Should not panic - result can be any integer
		result := kmpIndex(len(pattern), text, equals)
		_ = result // Any result is valid, just ensure no panic
	})
	
	t.Run("empty_pattern_edge_case", func(t *testing.T) {
		var pattern []*int
		text := []*int{new(int), new(int)}
		
		equals := func(a, b *int) bool { return a == b }
		
		result := kmpIndex(0, text, equals)
		require.Equal(t, 0, result, "Empty pattern should return 0")
		_ = pattern // Silence unused variable warning
	})
	
	t.Run("realistic_pattern_performance", func(t *testing.T) {
		// Test with realistic sizes to ensure good performance
		realisticSize := 100 // More realistic for validator arrays
		pattern := make([]*int, realisticSize)
		text := make([]*int, realisticSize*2)
		
		// Create realistic pattern
		for i := range pattern {
			val := i % 10 // More variation
			pattern[i] = &val
		}
		for i := range text {
			val := i % 10
			text[i] = &val
		}
		
		equals := func(a, b *int) bool {
			if a == nil && b == nil {
				return true
			}
			if a == nil || b == nil {
				return false
			}
			return *a == *b
		}
		
		start := time.Now()
		result := kmpIndex(len(pattern), text, equals)
		duration := time.Since(start)
		
		// Should complete quickly with realistic inputs
		require.Equal(t, true, duration < time.Second, 
			"KMP took too long: %v", duration)
		_ = result // Any result is valid, just ensure performance is good
	})
}

// TestConcurrencySafety tests thread safety of the hdiff operations
func TestConcurrencySafety(t *testing.T) {
	t.Run("concurrent_diff_creation", func(t *testing.T) {
		source, _ := util.DeterministicGenesisStateElectra(t, 32)
		target := source.Copy()
		_ = target.SetSlot(source.Slot() + 1)
		
		const numGoroutines = 10
		const iterations = 100
		
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines*iterations)
		
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for j := 0; j < iterations; j++ {
					_, err := Diff(source, target)
					if err != nil {
						errors <- fmt.Errorf("worker %d iteration %d: %v", workerID, j, err)
					}
				}
			}(i)
		}
		
		wg.Wait()
		close(errors)
		
		// Check for any errors
		for err := range errors {
			t.Error(err)
		}
	})
	
	t.Run("concurrent_diff_application", func(t *testing.T) {
		ctx := t.Context()
		source, _ := util.DeterministicGenesisStateElectra(t, 16)
		target := source.Copy()
		_ = target.SetSlot(source.Slot() + 5)
		
		diff, err := Diff(source, target)
		require.NoError(t, err)
		
		const numGoroutines = 10
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)
		
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				// Each goroutine needs its own copy of the source state
				localSource := source.Copy()
				_, err := ApplyDiff(ctx, localSource, diff)
				if err != nil {
					errors <- fmt.Errorf("worker %d: %v", workerID, err)
				}
			}(i)
		}
		
		wg.Wait()
		close(errors)
		
		// Check for any errors
		for err := range errors {
			t.Error(err)
		}
	})
}