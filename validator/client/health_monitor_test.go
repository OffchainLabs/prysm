package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/OffchainLabs/prysm/v6/config/params"
	validatormock "github.com/OffchainLabs/prysm/v6/testing/validator-mock"
)

// TestHealthMonitor_IsHealthy_Concurrency tests thread-safety of IsHealthy.
func TestHealthMonitor_IsHealthy_Concurrency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := validatormock.NewMockValidator(ctrl)
	// inside the test
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	t.Cleanup(parentCancel)

	// Expectation for newHealthMonitor's FindHealthyHost call
	mockValidator.EXPECT().FindHealthyHost(gomock.Any()).Return(true).Times(1)

	monitor := newHealthMonitor(parentCtx, parentCancel, 3, mockValidator)
	require.NotNil(t, monitor)
	monitor.Start()
	time.Sleep(100 * time.Millisecond)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.True(t, monitor.IsHealthy())
		}()
	}
	wg.Wait()

	// Test when isHealthy is false
	monitor.Lock()
	monitor.isHealthy = false
	monitor.Unlock()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.False(t, monitor.IsHealthy())
		}()
	}
	wg.Wait()
}

// TestHealthMonitor_PerformHealthCheck tests the core logic of a single health check.
func TestHealthMonitor_PerformHealthCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := validatormock.NewMockValidator(ctrl)

	tests := []struct {
		expectStatusUpdate     bool // true if healthyCh should receive a new, different status
		expectCancelCalled     bool
		expectedIsHealthy      bool
		findHealthyHostReturns bool
		initialIsHealthy       bool
		expectedFails          int
		maxFails               int
		initialFails           int
		name                   string
	}{
		{
			name:                   "Becomes Unhealthy",
			initialIsHealthy:       true,
			initialFails:           0,
			maxFails:               3,
			findHealthyHostReturns: false,
			expectedIsHealthy:      false,
			expectedFails:          1,
			expectCancelCalled:     false,
			expectStatusUpdate:     true,
		},
		{
			name:                   "Becomes Healthy",
			initialIsHealthy:       false,
			initialFails:           1,
			maxFails:               3,
			findHealthyHostReturns: true,
			expectedIsHealthy:      true,
			expectedFails:          0,
			expectCancelCalled:     false,
			expectStatusUpdate:     true,
		},
		{
			name:                   "Remains Healthy",
			initialIsHealthy:       true,
			initialFails:           0,
			maxFails:               3,
			findHealthyHostReturns: true,
			expectedIsHealthy:      true,
			expectedFails:          0,
			expectCancelCalled:     false,
			expectStatusUpdate:     false, // Status did not change
		},
		{
			name:                   "Remains Unhealthy",
			initialIsHealthy:       false,
			initialFails:           1,
			maxFails:               3,
			findHealthyHostReturns: false,
			expectedIsHealthy:      false,
			expectedFails:          2,
			expectCancelCalled:     false,
			expectStatusUpdate:     false, // Status did not change
		},
		{
			name:                   "Max Fails Reached - Stays Unhealthy and Cancels",
			initialIsHealthy:       false,
			initialFails:           2, // One fail away from maxFails
			maxFails:               2,
			findHealthyHostReturns: false,
			expectedIsHealthy:      false,
			expectedFails:          2,
			expectCancelCalled:     true,
			expectStatusUpdate:     false, // Status was already false, no new update sent before cancel
		},
		{
			name:                   "MaxFails is 0 - Remains Unhealthy, No Cancel",
			initialIsHealthy:       false,
			initialFails:           100, // Arbitrarily high
			maxFails:               0,   // Infinite
			findHealthyHostReturns: false,
			expectedIsHealthy:      false,
			expectedFails:          100,
			expectCancelCalled:     false,
			expectStatusUpdate:     false,
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

			monitor := &healthMonitor{
				ctx:             monitorCtx,         // Context for the monitor's operations
				cancel:          testCancelCallback, // This is m.cancel()
				v:               mockValidator,
				maxFails:        tt.maxFails,
				healthyCh:       make(chan bool, 1),
				fails:           tt.initialFails,
				isHealthy:       tt.initialIsHealthy,
				healthEventFeed: new(event.Feed),
			}
			monitor.healthEventFeed.Subscribe(monitor.healthyCh)

			mockValidator.EXPECT().FindHealthyHost(gomock.Any()).Return(tt.findHealthyHostReturns)

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := validatormock.NewMockValidator(ctrl)
	monitorCtx, monitorCancelFunc := context.WithCancel(context.Background())

	originalSecPerSlot := params.BeaconConfig().SecondsPerSlot
	params.BeaconConfig().SecondsPerSlot = 1 // 1 sec interval for test
	defer func() {
		params.BeaconConfig().SecondsPerSlot = originalSecPerSlot
		monitorCancelFunc() // Ensure monitor context is cleaned up
	}()

	monitor := newHealthMonitor(monitorCtx, monitorCancelFunc, 3, mockValidator)
	require.NotNil(t, monitor)

	ch := monitor.HealthyChan()
	require.NotNil(t, ch)

	first := mockValidator.EXPECT().
		FindHealthyHost(gomock.Any()).
		Return(true).Times(1)

	mockValidator.EXPECT().
		FindHealthyHost(gomock.Any()).
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
