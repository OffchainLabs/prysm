package health

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"go.uber.org/mock/gomock"
)

var (
	_ = Node(&MockHealthClient{})
)

func TestNodeHealth_IsHealthy(t *testing.T) {
	tests := []struct {
		name      string
		isHealthy bool
		want      bool
	}{
		{"initially healthy", true, true},
		{"initially unhealthy", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NodeHealthTracker{
				isHealthy:  &tt.isHealthy,
				healthChan: make(chan bool, 1),
			}
			if got := n.IsHealthy(context.Background()); got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeHealth_UpdateNodeHealth(t *testing.T) {
	tests := []struct {
		name       string
		initial    bool // Initial health status
		newStatus  bool // Status to update to
		shouldSend bool // Should a message be sent through the channel
	}{
		{"healthy to unhealthy", true, false, true},
		{"unhealthy to healthy", false, true, true},
		{"remain healthy", true, true, false},
		{"remain unhealthy", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := NewMockHealthClient(ctrl)
			client.EXPECT().IsHealthy(gomock.Any()).Return(tt.newStatus)
			n := &NodeHealthTracker{
				isHealthy:  &tt.initial,
				node:       client,
				healthChan: make(chan bool, 1),
			}

			s := n.CheckHealth(context.Background())
			// Check if health status was updated
			if s != tt.newStatus {
				t.Errorf("UpdateNodeHealth() failed to update isHealthy from %v to %v", tt.initial, tt.newStatus)
			}

			select {
			case status := <-n.HealthUpdates():
				if !tt.shouldSend {
					t.Errorf("UpdateNodeHealth() unexpectedly sent status %v to HealthCh", status)
				} else if status != tt.newStatus {
					t.Errorf("UpdateNodeHealth() sent wrong status %v, want %v", status, tt.newStatus)
				}
			default:
				if tt.shouldSend {
					t.Error("UpdateNodeHealth() did not send any status to HealthCh when expected")
				}
			}
		})
	}
}

func TestNodeHealth_Concurrency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := NewMockHealthClient(ctrl)
	n := NewTracker(client)
	var wg sync.WaitGroup

	// Number of goroutines to spawn for both reading and writing
	numGoroutines := 6

	wg.Add(numGoroutines * 2) // for readers and writers

	// Concurrently update health status
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			client.EXPECT().IsHealthy(gomock.Any()).Return(false).Times(1)
			n.CheckHealth(context.Background())
			client.EXPECT().IsHealthy(gomock.Any()).Return(true).Times(1)
			n.CheckHealth(context.Background())
		}()
	}

	// Concurrently read health status
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = n.IsHealthy(context.Background()) // Just read the value
		}()
	}

	wg.Wait() // Wait for all goroutines to finish
}

// MockHealthClient is a mock of HealthClient interface.
type MockHealthClient struct {
	ctrl     *gomock.Controller
	recorder *MockHealthClientMockRecorder
	sync.Mutex
}

// MockHealthClientMockRecorder is the mock recorder for MockHealthClient.
type MockHealthClientMockRecorder struct {
	mock *MockHealthClient
}

// IsHealthy mocks base method.
func (m *MockHealthClient) IsHealthy(arg0 context.Context) bool {
	m.Lock()
	defer m.Unlock()
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsHealthy", arg0)
	ret0, ok := ret[0].(bool)
	if !ok {
		return false
	}
	return ret0
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockHealthClient) EXPECT() *MockHealthClientMockRecorder {
	return m.recorder
}

// IsHealthy indicates an expected call of IsHealthy.
func (mr *MockHealthClientMockRecorder) IsHealthy(arg0 any) *gomock.Call {
	mr.mock.Lock()
	defer mr.mock.Unlock()
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsHealthy", reflect.TypeOf((*MockHealthClient)(nil).IsHealthy), arg0)
}

// NewMockHealthClient creates a new mock instance.
func NewMockHealthClient(ctrl *gomock.Controller) *MockHealthClient {
	mock := &MockHealthClient{ctrl: ctrl}
	mock.recorder = &MockHealthClientMockRecorder{mock}
	return mock
}
