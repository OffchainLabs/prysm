package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/OffchainLabs/prysm/v6/config/params"
	validatormock "github.com/OffchainLabs/prysm/v6/testing/validator-mock"
)

// TestNewHealthMonitor verifies the initialization logic of the health monitor.
func TestNewHealthMonitor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := validatormock.NewMockValidator(ctrl)
	_, parentCancelFunc := context.WithCancel(context.Background())
	defer parentCancelFunc() // Ensure cleanup for this top-level context

	tests := []struct {
		name              string
		maxFails          int
		findHealthyHost   bool
		expectedIsHealthy bool
		expectedFails     int
	}{
		{
			name:              "Initially Healthy",
			maxFails:          3,
			findHealthyHost:   true,
			expectedIsHealthy: true,
			expectedFails:     0,
		},
		{
			name:              "Initially Unhealthy",
			maxFails:          3,
			findHealthyHost:   false,
			expectedIsHealthy: false,
			expectedFails:     1,
		},
		{
			name:              "MaxFails 0, Initially Healthy",
			maxFails:          0, // infinite retries
			findHealthyHost:   true,
			expectedIsHealthy: true,
			expectedFails:     0,
		},
		{
			name:              "MaxFails 0, Initially Unhealthy",
			maxFails:          0, // infinite retries
			findHealthyHost:   false,
			expectedIsHealthy: false,
			expectedFails:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockValidator.EXPECT().FindHealthyHost(gomock.Any()).Return(tt.findHealthyHost)

			monitorTestCtx, monitorTestCancelFunc := context.WithCancel(context.Background())
			defer monitorTestCancelFunc()

			monitor := newHealthMonitor(monitorTestCtx, monitorTestCancelFunc, tt.maxFails, mockValidator)
			require.NotNil(t, monitor)

			assert.Equal(t, tt.expectedIsHealthy, monitor.IsHealthy())
			// Accessing fails directly for test validation of internal state.
			assert.Equal(t, tt.expectedFails, monitor.fails)

			// Check channel for initial prime value
			select {
			case healthyStatus := <-monitor.HealthyChan():
				assert.Equal(t, tt.expectedIsHealthy, healthyStatus)
			case <-time.After(100 * time.Millisecond):
				t.Fatal("Expected initial status on HealthyChan, but timed out")
			}
		})
	}
}

// TestHealthMonitor_IsHealthy_Concurrency tests thread-safety of IsHealthy.
func TestHealthMonitor_IsHealthy_Concurrency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := validatormock.NewMockValidator(ctrl)
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	// Expectation for newHealthMonitor's FindHealthyHost call
	mockValidator.EXPECT().FindHealthyHost(gomock.Any()).Return(true).Times(1)

	monitor := newHealthMonitor(parentCtx, parentCancel, 3, mockValidator)
	require.NotNil(t, monitor)
	<-monitor.HealthyChan() // Drain initial value from newHealthMonitor

	var wg sync.WaitGroup
	numGoroutines := 10

	// Test when isHealthy is true
	monitor.Lock()
	monitor.isHealthy = true
	monitor.Unlock()

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
		name                   string
		initialIsHealthy       bool
		initialFails           int
		maxFails               int
		findHealthyHostReturns bool
		expectedIsHealthy      bool
		expectedFails          int
		expectCancelCalled     bool
		expectStatusUpdate     bool // true if healthyCh should receive a new, different status
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
			maxFails:               3,
			findHealthyHostReturns: false,
			expectedIsHealthy:      false,
			expectedFails:          3,
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
			expectedFails:          101,
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
				ctx:       monitorCtx,         // Context for the monitor's operations
				cancel:    testCancelCallback, // This is m.cancel()
				v:         mockValidator,
				maxFails:  tt.maxFails,
				healthyCh: make(chan bool, 1),
				fails:     tt.initialFails,
				isHealthy: tt.initialIsHealthy,
			}

			mockValidator.EXPECT().FindHealthyHost(gomock.Any()).Return(tt.findHealthyHostReturns)

			monitor.performHealthCheck()

			assert.Equal(t, tt.expectedIsHealthy, monitor.IsHealthy(), "isHealthy mismatch")
			assert.Equal(t, tt.expectedFails, monitor.fails, "fails count mismatch")
			assert.Equal(t, tt.expectCancelCalled, actualCancelFuncCalled, "cancelCalled mismatch")

			if tt.expectStatusUpdate {
				select {
				case newStatus := <-monitor.HealthyChan():
					assert.Equal(t, tt.expectedIsHealthy, newStatus, "HealthyChan received unexpected status")
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Expected status update on HealthyChan, but timed out")
				}
			} else {
				select {
				case status := <-monitor.HealthyChan():
					// If status didn't change, but was re-sent due to channel drain logic (not applicable here as we test single performHealthCheck)
					// For this test, if expectStatusUpdate is false, nothing should be sent.
					t.Fatalf("Did not expect status update on HealthyChan, but got %v", status)
				default:
					// Expected: no update if status didn't change
				}
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

	// 1. For newHealthMonitor: Initial status is true
	mockValidator.EXPECT().FindHealthyHost(gomock.Any()).Return(true).Times(1)
	monitor := newHealthMonitor(monitorCtx, monitorCancelFunc, 3, mockValidator)
	require.NotNil(t, monitor)

	ch := monitor.HealthyChan()
	require.NotNil(t, ch)

	// Consume initial prime value (true)
	select {
	case status := <-ch:
		assert.True(t, status, "Expected initial status to be true")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for initial status")
	}

	mockValidator.EXPECT().FindHealthyHost(gomock.Any()).Return(false).AnyTimes()

	monitor.Start()

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
