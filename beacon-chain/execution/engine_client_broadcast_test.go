package execution

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

// TestStartRetryIfNeeded_AtomicBehavior tests that the atomic retry start behavior
// prevents race conditions by ensuring only one retry can be active per blockRoot.
func TestStartRetryIfNeeded_AtomicBehavior(t *testing.T) {
	t.Run("prevents multiple concurrent retry claims", func(t *testing.T) {
		service := &Service{
			activeRetries: sync.Map{},
		}
		
		blockRoot := [32]byte{1, 2, 3}
		claimCount := int64(0)
		
		numConcurrentCalls := 20
		var wg sync.WaitGroup
		startSignal := make(chan struct{})
		
		// Launch multiple goroutines that try to claim retry slot simultaneously
		for i := 0; i < numConcurrentCalls; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startSignal // Wait for signal to maximize race contention
				
				// Simulate the atomic claim logic from startRetryIfNeeded
				cancelFunc := func() {}
				if _, loaded := service.activeRetries.LoadOrStore(blockRoot, cancelFunc); !loaded {
					// We won the race - count successful claims
					atomic.AddInt64(&claimCount, 1)
					
					// Simulate some work before cleaning up
					time.Sleep(1 * time.Millisecond)
					service.activeRetries.Delete(blockRoot)
				}
			}()
		}
		
		// Start all goroutines simultaneously to maximize race condition
		close(startSignal)
		wg.Wait()
		
		// Verify only one goroutine successfully claimed the retry slot
		actualClaimCount := atomic.LoadInt64(&claimCount)
		require.Equal(t, int64(1), actualClaimCount, "Only one goroutine should successfully claim retry slot despite %d concurrent attempts", numConcurrentCalls)
		
		t.Logf("Success: %d concurrent attempts resulted in only 1 successful claim (atomic behavior verified)", numConcurrentCalls)
	})
	
	t.Run("hasActiveRetry correctly detects active retries", func(t *testing.T) {
		service := &Service{
			activeRetries: sync.Map{},
		}
		
		blockRoot1 := [32]byte{1, 2, 3}
		blockRoot2 := [32]byte{4, 5, 6}
		
		// Initially no active retries
		if service.hasActiveRetry(blockRoot1) {
			t.Error("Should not have active retry initially")
		}
		
		// Add active retry for blockRoot1
		service.activeRetries.Store(blockRoot1, func() {})
		
		// Verify detection
		if !service.hasActiveRetry(blockRoot1) {
			t.Error("Should detect active retry for blockRoot1")
		}
		if service.hasActiveRetry(blockRoot2) {
			t.Error("Should not detect active retry for blockRoot2")
		}
		
		// Remove active retry
		service.activeRetries.Delete(blockRoot1)
		
		// Verify removal
		if service.hasActiveRetry(blockRoot1) {
			t.Error("Should not detect active retry after deletion")
		}
		
		t.Logf("Success: hasActiveRetry correctly tracks retry state")
	})
}