package health

import (
	"context"
	"reflect"
	"sync"

	"go.uber.org/mock/gomock"
)

// NewMockHealthClient creates a new mock instance.
func NewMockHealthClient(ctrl *gomock.Controller) *MockHealthClient {
	mock := &MockHealthClient{ctrl: ctrl}
	mock.recorder = &MockHealthClientMockRecorder{mock}
	return mock
}

// MockHealthClient is a mock of HealthClient interface.
type MockHealthClient struct {
	ctrl     *gomock.Controller
	recorder *MockHealthClientMockRecorder
	Health   bool
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

func (m *MockHealthClient) HealthUpdates() <-chan bool {
	ch := make(chan bool, 2)
	ch <- m.Health
	return ch
}

func (m *MockHealthClient) CheckHealth(_ context.Context) bool {
	return m.Health
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
