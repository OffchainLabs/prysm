package client

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/OffchainLabs/prysm/v7/config/params"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
)

// TestHealthMonitor_IsHealthy_Concurrency tests thread-safety of IsHealthy.
func TestHealthMonitor_IsHealthy_Concurrency(t *testing.T) {
	vc := healthTestClient(t)
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	t.Cleanup(parentCancel)

	// Expectation for newHealthMonitor's EnsureReady call
	vc.EXPECT().EnsureReady(gomock.Any()).Return(true).Times(1)

	monitor := newHealthMonitor(parentCtx, parentCancel, 3, vc)
	require.NotNil(t, monitor)
	monitor.Start()
	time.Sleep(100 * time.Millisecond)

	var wg sync.WaitGroup
	numGoroutines := 10

	for range numGoroutines {
		wg.Go(func() {
			assert.True(t, monitor.IsHealthy())
		})
	}
	wg.Wait()

	// Test when isHealthy is false
	monitor.Lock()
	monitor.isHealthy = false
	monitor.Unlock()

	for range numGoroutines {
		wg.Go(func() {
			assert.False(t, monitor.IsHealthy())
		})
	}
	wg.Wait()
}

// TestHealthMonitor_PerformHealthCheck tests the core logic of a single health check.
func TestHealthMonitor_PerformHealthCheck(t *testing.T) {
	tests := []struct {
		expectStatusUpdate bool // true if healthyCh should receive a new, different status
		expectCancelCalled bool
		expectedIsHealthy  bool
		ensureReadyReturns bool
		initialIsHealthy   bool
		expectedFails      int
		maxFails           int
		initialFails       int
		name               string
	}{
		{
			name:               "Becomes Unhealthy",
			initialIsHealthy:   true,
			initialFails:       0,
			maxFails:           3,
			ensureReadyReturns: false,
			expectedIsHealthy:  false,
			expectedFails:      1,
			expectCancelCalled: false,
			expectStatusUpdate: true,
		},
		{
			name:               "Becomes Healthy",
			initialIsHealthy:   false,
			initialFails:       1,
			maxFails:           3,
			ensureReadyReturns: true,
			expectedIsHealthy:  true,
			expectedFails:      0,
			expectCancelCalled: false,
			expectStatusUpdate: true,
		},
		{
			name:               "Remains Healthy",
			initialIsHealthy:   true,
			initialFails:       0,
			maxFails:           3,
			ensureReadyReturns: true,
			expectedIsHealthy:  true,
			expectedFails:      0,
			expectCancelCalled: false,
			expectStatusUpdate: false, // Status did not change
		},
		{
			name:               "Remains Unhealthy",
			initialIsHealthy:   false,
			initialFails:       1,
			maxFails:           3,
			ensureReadyReturns: false,
			expectedIsHealthy:  false,
			expectedFails:      2,
			expectCancelCalled: false,
			expectStatusUpdate: false, // Status did not change
		},
		{
			name:               "Max Fails Reached - Stays Unhealthy and Cancels",
			initialIsHealthy:   false,
			initialFails:       2, // One fail away from maxFails
			maxFails:           2,
			ensureReadyReturns: false,
			expectedIsHealthy:  false,
			expectedFails:      2,
			expectCancelCalled: true,
			expectStatusUpdate: false, // Status was already false, no new update sent before cancel
		},
		{
			name:               "MaxFails is 0 - Remains Unhealthy, No Cancel",
			initialIsHealthy:   false,
			initialFails:       100, // Arbitrarily high
			maxFails:           0,   // Infinite
			ensureReadyReturns: false,
			expectedIsHealthy:  false,
			expectedFails:      100,
			expectCancelCalled: false,
			expectStatusUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitorCtx, monitorCancelFunc := context.WithCancel(context.Background())
			var actualCancelFuncCalled bool
			testCancelCallback := func() {
				actualCancelFuncCalled = true
				monitorCancelFunc() // Propagate to monitorCtx if needed for other parts
			}

			vc := healthTestClient(t)
			monitor := &healthMonitor{
				ctx:             monitorCtx,         // Context for the monitor's operations
				cancel:          testCancelCallback, // This is m.cancel()
				client:          vc,
				maxFails:        tt.maxFails,
				healthyCh:       make(chan bool, 1),
				fails:           tt.initialFails,
				isHealthy:       tt.initialIsHealthy,
				healthEventFeed: new(event.Feed),
			}
			monitor.healthEventFeed.Subscribe(monitor.healthyCh)

			vc.EXPECT().EnsureReady(gomock.Any()).Return(tt.ensureReadyReturns)

			monitor.performHealthCheck()

			assert.Equal(t, tt.expectedIsHealthy, monitor.IsHealthy(), "isHealthy mismatch")
			assert.Equal(t, tt.expectedFails, monitor.fails, "fails count mismatch")
			assert.Equal(t, tt.expectCancelCalled, actualCancelFuncCalled, "cancelCalled mismatch")

			if tt.expectStatusUpdate {
				assert.Eventually(t, func() bool {
					select {
					case s := <-monitor.HealthyChan():
						return s == tt.expectedIsHealthy
					default:
						return false
					}
				}, 100*time.Millisecond, 10*time.Millisecond) // wait, poll
			} else {
				assert.Never(t, func() bool {
					select {
					case <-monitor.HealthyChan():
						return true // received something: fail
					default:
						return false
					}
				}, 100*time.Millisecond, 10*time.Millisecond)
			}
			if !actualCancelFuncCalled {
				monitorCancelFunc() // Clean up context if not cancelled by test logic
			}
		})
	}
}

// TestHealthMonitor_HealthyChan_ReceivesUpdates tests channel behavior.
func TestHealthMonitor_HealthyChan_ReceivesUpdates(t *testing.T) {
	vc := healthTestClient(t)
	monitorCtx, monitorCancelFunc := context.WithCancel(context.Background())

	originalSlotDurationMs := params.BeaconConfig().SlotDurationMilliseconds
	params.BeaconConfig().SlotDurationMilliseconds = 1000 // 1 sec interval for test
	defer func() {
		params.BeaconConfig().SlotDurationMilliseconds = originalSlotDurationMs
		monitorCancelFunc() // Ensure monitor context is cleaned up
	}()

	monitor := newHealthMonitor(monitorCtx, monitorCancelFunc, 3, vc)
	require.NotNil(t, monitor)

	ch := monitor.HealthyChan()
	require.NotNil(t, ch)

	first := vc.EXPECT().
		EnsureReady(gomock.Any()).
		Return(true).Times(1)

	vc.EXPECT().
		EnsureReady(gomock.Any()).
		Return(false).
		AnyTimes().
		After(first)

	monitor.Start()

	// Consume initial prime value (true)
	select {
	case status := <-ch:
		assert.True(t, status, "Expected initial status to be true")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for initial status")
	}

	// Expect 'false' from the first check in Start's loop
	select {
	case status := <-ch:
		assert.False(t, status, "Expected status to change to false")
	case <-time.After(2 * time.Second): // Timeout for tick + processing
		t.Fatal("Timeout waiting for status change to false")
	}

	// 4. Stop the monitor
	monitor.Stop() // This calls monitorCancelFunc
}

func TestHealthMonitor_WaitForHealthy(t *testing.T) {
	originalSlotDurationMs := params.BeaconConfig().SlotDurationMilliseconds
	params.BeaconConfig().SlotDurationMilliseconds = 20
	defer func() { params.BeaconConfig().SlotDurationMilliseconds = originalSlotDurationMs }()

	t.Run("a verdict recorded before the call does not release it", func(t *testing.T) {
		monitor, healthy := testHealthMonitor(t)
		healthy.Store(true)
		monitor.performHealthCheck()
		require.True(t, monitor.IsHealthy())

		done := make(chan error, 1)
		go func() { done <- monitor.WaitForHealthy(context.Background()) }()
		select {
		case <-done:
			t.Fatal("returned on a verdict recorded before the call")
		case <-time.After(100 * time.Millisecond):
		}
		require.Eventually(t, func() bool {
			monitor.performHealthCheck()
			select {
			case err := <-done:
				require.NoError(t, err)
				return true
			default:
				return false
			}
		}, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("blocks until a probe reports healthy", func(t *testing.T) {
		monitor, healthy := testHealthMonitor(t)
		monitor.Start()
		flipped := make(chan time.Time, 1)
		go func() {
			time.Sleep(150 * time.Millisecond)
			healthy.Store(true)
			flipped <- time.Now()
		}()
		require.NoError(t, monitor.WaitForHealthy(context.Background()))
		assert.False(t, time.Now().Before(<-flipped), "returned before the monitor became healthy")
	})

	t.Run("returns the context error when canceled", func(t *testing.T) {
		monitor, _ := testHealthMonitor(t)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		require.ErrorIs(t, monitor.WaitForHealthy(ctx), context.Canceled)
	})

	t.Run("a returned waiter does not block later probes", func(t *testing.T) {
		monitor, _ := testHealthMonitor(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, monitor.WaitForHealthy(ctx), context.Canceled)

		// The verdict send is synchronous, so a stale subscription would stall here.
		probed := make(chan bool, 1)
		go func() {
			monitor.performHealthCheck()
			monitor.performHealthCheck()
			probed <- true
		}()
		select {
		case <-probed:
		case <-time.After(time.Second):
			t.Fatal("probes blocked on a waiter that already returned")
		}
	})
}

func healthTestClient(t *testing.T) *validatormock.MockValidatorClient {
	vc := validatormock.NewMockValidatorClient(gomock.NewController(t))
	vc.EXPECT().Host().Return("http://localhost:3500").AnyTimes()
	return vc
}

// testHealthMonitor returns an unstarted monitor whose probes report the flag's
// current value; status events are drained so probes never block on them.
func testHealthMonitor(t *testing.T) (*healthMonitor, *atomic.Bool) {
	var healthy atomic.Bool
	vc := healthTestClient(t)
	vc.EXPECT().EnsureReady(gomock.Any()).DoAndReturn(func(context.Context) bool { return healthy.Load() }).AnyTimes()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	monitor := newHealthMonitor(ctx, cancel, 0, vc)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-monitor.HealthyChan():
			}
		}
	}()
	return monitor, &healthy
}
