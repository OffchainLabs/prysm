package startup

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

// MockNower is a mock implementation of the Nower interface for use in tests.
type MockNower struct {
	t time.Time
}

// Now satisfies the Nower interface using a mocked time value
func (m *MockNower) Now() time.Time {
	return m.t
}

// SetSlot sets the current time to the start of the given slot.
func (m *MockNower) SetSlot(t *testing.T, c *Clock, s primitives.Slot) {
	now, err := slots.StartTime(c.GenesisTime(), s)
	if err != nil {
		t.Fatalf("failed to set slot: %v", err)
	}
	m.t = now
}

// Set sets the current time to the given time.
func (m *MockNower) Set(now time.Time) {
	m.t = now
}
