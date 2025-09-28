package sync

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"google.golang.org/protobuf/proto"
)

// TestSubscriptionCleanup_MissingRemoveTopic tests the following bug:
// When a subscription's message loop fails and sub.Cancel() is called,
// removeTopic() is NOT called, leaving stale entries in subTopics map.
// This likely causes memory leaks and prevents resubscription (missed attestations).
func TestSubscriptionCleanup_MissingRemoveTopic(t *testing.T) {
	t.Run("memory leak with repeated failures", func(t *testing.T) {
		// This test verifies that removeTopic() is called when subscription fails
		// Fresh setup for this subtest
		p2pService := p2ptest.NewTestP2P(t)
		gt := time.Now()
		vr := [32]byte{'A'}

		r := &Service{
			ctx: context.Background(),
			cfg: &config{
				p2p:         p2pService,
				initialSync: &mockSync.Sync{IsSyncing: false},
				chain: &mockChain.ChainService{
					ValidatorsRoot: vr,
					Genesis:        gt,
				},
				clock: startup.NewClock(gt, vr),
			},
			subHandler:   newSubTopicHandler(),
			chainStarted: abool.New(),
		}
		markInitSyncComplete(t, r)

		digest, err := r.currentForkDigest()
		require.NoError(t, err)
		p2pService.Digest = digest

		getMapSize := func() int {
			r.subHandler.RLock()
			defer r.subHandler.RUnlock()
			return len(r.subHandler.subTopics)
		}

		baseTopic := "/eth2/%x/voluntary_exit"

		// Do one cycle: subscribe, cancel, check cleanup
		iterCtx, iterCancel := context.WithCancel(context.Background())
		r.ctx = iterCtx

		handler := func(ctx context.Context, msg proto.Message) error {
			return nil
		}

		r.markForChainStart()

		// Subscribe
		sub := r.subscribeWithBase(baseTopic, r.noopValidator, handler)
		require.NotNil(t, sub, "First subscription should succeed")

		// Verify subscribed
		sizeAfterSubscribe := getMapSize()
		require.Equal(t, 1, sizeAfterSubscribe, "Should have 1 entry after subscribe")

		// Cancel to simulate failure
		iterCancel()
		time.Sleep(300 * time.Millisecond)

		// Check cleanup happened - this is the core fix verification
		sizeAfterCancel := getMapSize()
		if sizeAfterCancel != 0 {
			t.Errorf("After context cancellation, subTopics has %d entries (expected 0). "+
				"removeTopic() should have been called at line 420.",
				sizeAfterCancel)
		} else {
			t.Logf("SUCCESS: Cleanup working correctly - map size is 0 after cancellation")
		}
	})
}

// TestConcurrentSubscription_RaceCondition tests the following bug:
// Multiple goroutines can pass topicExists() check simultaneously
// before any calls addTopic(), causing duplicate subscriptions.
func TestConcurrentSubscription_RaceCondition(t *testing.T) {
	tests := []struct {
		name          string
		numGoroutines int
		iterations    int
		useBarrier    bool
	}{
		{
			name:          "two concurrent",
			numGoroutines: 2,
			iterations:    20,
			useBarrier:    true,
		},
		{
			name:          "five concurrent",
			numGoroutines: 5,
			iterations:    15,
			useBarrier:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duplicateDetected := 0

			for iter := 0; iter < tt.iterations; iter++ {
				// Fresh setup for each iteration
				p2pService := p2ptest.NewTestP2P(t)
				gt := time.Now()
				vr := [32]byte{'A'}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

				r := &Service{
					ctx: ctx,
					cfg: &config{
						p2p:         p2pService,
						initialSync: &mockSync.Sync{IsSyncing: false},
						chain: &mockChain.ChainService{
							ValidatorsRoot: vr,
							Genesis:        gt,
						},
						clock: startup.NewClock(gt, vr),
					},
					subHandler:   newSubTopicHandler(),
					chainStarted: abool.New(),
				}
				markInitSyncComplete(t, r)

				digest, err := r.currentForkDigest()
				require.NoError(t, err)
				p2pService.Digest = digest

				baseTopic := "/eth2/%x/voluntary_exit"

				r.markForChainStart()

				// Track successful subscriptions
				successfulSubs := atomic.Int32{}
				checksPassed := atomic.Int32{}

				// Barrier to synchronize goroutine starts
				var barrier sync.WaitGroup
				if tt.useBarrier {
					barrier.Add(tt.numGoroutines)
				}
				startSignal := make(chan struct{})

				var wg sync.WaitGroup

				// Launch concurrent subscription attempts
				for i := 0; i < tt.numGoroutines; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						if tt.useBarrier {
							barrier.Done()
							barrier.Wait()
						}

						<-startSignal

						// Attempt subscription
						// ideally only one goroutine should get a non-nil subscription
						handler := func(ctx context.Context, msg proto.Message) error {
							return nil
						}

						sub := r.subscribeWithBase(baseTopic, r.noopValidator, handler)
						if sub != nil {
							successfulSubs.Add(1)
						}
						// Count how many goroutines attempted (for stats)
						checksPassed.Add(1)
					}()
				}

				// Wait for all goroutines to be ready
				if tt.useBarrier {
					barrier.Wait()
				}
				time.Sleep(10 * time.Millisecond)

				// Start all goroutines simultaneously
				close(startSignal)

				// Wait for completion
				wg.Wait()
				time.Sleep(200 * time.Millisecond)

				// Check results
				subs := successfulSubs.Load()
				attempts := checksPassed.Load()

				r.subHandler.RLock()
				finalMapSize := len(r.subHandler.subTopics)
				r.subHandler.RUnlock()

				// ideally only ONE goroutine should successfully subscribe
				// If more than one succeeds, a race condition exists
				if subs > 1 {
					duplicateDetected++
					t.Logf("Iteration %d: RACE DETECTED - %d goroutines attempted, "+
						"%d successful subscriptions (expected 1), final map size: %d",
						iter, attempts, subs, finalMapSize)
				}

				// The map should have exactly 0 or 1 entry
				if finalMapSize > 1 {
					t.Errorf("Iteration %d: INCONSISTENT STATE - map has %d entries (expected 0-1). "+
						"This indicates multiple goroutines subscribed concurrently.",
						iter, finalMapSize)
				}

				// Cleanup
				cancel()
				r.subHandler.Lock()
				for topic := range r.subHandler.subTopics {
					sub := r.subHandler.subTopics[topic]
					if sub != nil {
						sub.Cancel()
					}
					delete(r.subHandler.subTopics, topic)
				}
				r.subHandler.Unlock()
			}

			if duplicateDetected > 0 {
				racePercentage := float64(duplicateDetected) / float64(tt.iterations) * 100
				t.Errorf("RACE CONDITION EXISTS in %d/%d iterations (%.1f%%). "+
					"Multiple goroutines successfully subscribed (only 1 expected). ",
					duplicateDetected, tt.iterations, racePercentage)
			} else {
				t.Logf("SUCCESS: No Race condition! Only 1 subscription succeeded in all %d iterations", tt.iterations)
			}
		})
	}
}

// TestMemoryGrowth_SubscriptionFailures demonstrates memory growth over time
func TestMemoryGrowth_SubscriptionFailures(t *testing.T) {
	p2pService := p2ptest.NewTestP2P(t)
	gt := time.Now()
	vr := [32]byte{'A'}

	r := &Service{
		ctx: context.Background(),
		cfg: &config{
			p2p:         p2pService,
			initialSync: &mockSync.Sync{IsSyncing: false},
			chain: &mockChain.ChainService{
				ValidatorsRoot: vr,
				Genesis:        gt,
			},
			clock: startup.NewClock(gt, vr),
		},
		subHandler:   newSubTopicHandler(),
		chainStarted: abool.New(),
	}
	markInitSyncComplete(t, r)

	digest, err := r.currentForkDigest()
	require.NoError(t, err)
	p2pService.Digest = digest

	baseTopic := "/eth2/%x/voluntary_exit"

	getMapSize := func() int {
		r.subHandler.RLock()
		defer r.subHandler.RUnlock()
		return len(r.subHandler.subTopics)
	}

	failures := 50
	var memStats runtime.MemStats

	runtime.ReadMemStats(&memStats)
	startAlloc := memStats.Alloc

	for i := 0; i < failures; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		r.ctx = ctx
		r.markForChainStart()

		handler := func(ctx context.Context, msg proto.Message) error {
			return nil
		}

		sub := r.subscribeWithBase(baseTopic, r.noopValidator, handler)
		if sub != nil {
			// Cancel immediately to simulate failure
			cancel()
			time.Sleep(100 * time.Millisecond)
		}

		if i%10 == 0 {
			runtime.ReadMemStats(&memStats)
			currentAlloc := memStats.Alloc
			growth := currentAlloc - startAlloc
			t.Logf("After %d failures: subTopics size=%d, heap growth=%d KB",
				i, getMapSize(), growth/1024)
		}
	}

	finalSize := getMapSize()
	runtime.ReadMemStats(&memStats)
	finalAlloc := memStats.Alloc

	t.Logf("Final results: %d subscription failures", failures)
	t.Logf("  subTopics map size: %d entries", finalSize)
	t.Logf("  Start heap: %d KB, Final heap: %d KB", startAlloc/1024, finalAlloc/1024)

	// With the bug, even one stale entry is a problem because it prevents resubscription
	if finalSize > 0 {
		t.Errorf("MEMORY LEAK / STALE ENTRY: After %d failures, %d stale entries remain in subTopics map (expected 0). "+
			"Even 1 stale entry prevents resubscription, causing missed attestations in production.",
			failures, finalSize)
	}

	// Check if heap grew significantly (handle wraparound by checking if finalAlloc >= startAlloc)
	if finalAlloc >= startAlloc {
		totalGrowth := finalAlloc - startAlloc
		if totalGrowth > 50*1024 { // 50 KB threshold
			t.Logf("NOTE: Heap grew by %d KB over %d failures. ",
				totalGrowth/1024, failures)
		}
	} else {
		t.Logf("NOTE: Heap decreased (GC ran), cannot measure growth accurately")
	}
}
