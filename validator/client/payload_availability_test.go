package client

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestPayloadAvailability_WaiterThenNotify(t *testing.T) {
	p := newPayloadAvailability()
	ch := p.waiter(1)
	require.Equal(t, false, isClosed(ch))
	p.notify(1)
	require.Equal(t, true, isClosed(ch))
}

func TestPayloadAvailability_NotifyThenWaiter(t *testing.T) {
	p := newPayloadAvailability()
	p.notify(1)
	require.Equal(t, true, isClosed(p.waiter(1)), "waiter registered after notify should observe availability")
}

func TestPayloadAvailability_MultipleWaitersSameSlot(t *testing.T) {
	p := newPayloadAvailability()
	a := p.waiter(1)
	b := p.waiter(1)
	p.notify(1)
	require.Equal(t, true, isClosed(a))
	require.Equal(t, true, isClosed(b))
}

func TestPayloadAvailability_NotifyIsIdempotent(t *testing.T) {
	p := newPayloadAvailability()
	p.waiter(1)
	p.notify(1)
	p.notify(1) // must not panic by double-closing.
}

func TestPayloadAvailability_PrunesOlderSlots(t *testing.T) {
	p := newPayloadAvailability()
	p.waiter(1)
	p.waiter(2)
	p.notify(3)
	p.mu.Lock()
	defer p.mu.Unlock()
	_, has1 := p.chans[1]
	_, has2 := p.chans[2]
	_, has3 := p.chans[3]
	require.Equal(t, false, has1)
	require.Equal(t, false, has2)
	require.Equal(t, true, has3)
}

func TestProcessEvent_ExecutionPayloadAvailableNotifiesWaiter(t *testing.T) {
	v := &validator{payloadAvailability: newPayloadAvailability()}
	ch := v.payloadAvailability.waiter(42)

	data, err := json.Marshal(&structs.ExecutionPayloadAvailableEvent{Slot: "42", BlockRoot: "0xabc"})
	require.NoError(t, err)
	v.ProcessEvent(t.Context(), &eventClient.Event{Type: eventClient.EventExecutionPayloadAvailable, Data: data})

	require.Equal(t, true, isClosed(ch))
}

func TestProcessEvent_ExecutionPayloadBadSlotDoesNotNotify(t *testing.T) {
	v := &validator{payloadAvailability: newPayloadAvailability()}
	ch := v.payloadAvailability.waiter(42)

	data, err := json.Marshal(&structs.ExecutionPayloadAvailableEvent{Slot: "not-a-number", BlockRoot: "0xabc"})
	require.NoError(t, err)
	v.ProcessEvent(t.Context(), &eventClient.Event{Type: eventClient.EventExecutionPayloadAvailable, Data: data})

	require.Equal(t, false, isClosed(ch))
}

func TestWaitForPayloadAvailableOrDeadline_ReturnsOnEvent(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	v := &validator{
		genesisTime:         time.Now().Add(time.Hour), // deadline far in the future.
		payloadAvailability: newPayloadAvailability(),
	}

	done := make(chan struct{})
	go func() {
		v.waitForPayloadAvailableOrDeadline(t.Context(), 0)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	v.payloadAvailability.notify(0)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not return after payload available event")
	}
}

func TestWaitForPayloadAvailableOrDeadline_ReturnsOnDeadline(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	v := &validator{
		genesisTime:         time.Time{}, // deadline already elapsed.
		payloadAvailability: newPayloadAvailability(),
	}

	done := make(chan struct{})
	go func() {
		v.waitForPayloadAvailableOrDeadline(t.Context(), primitives.Slot(1))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not return after deadline elapsed")
	}
}

func TestWaitForPayloadAvailableOrDeadline_ReturnsOnContextCancel(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	v := &validator{
		genesisTime:         time.Now().Add(time.Hour), // deadline far in the future.
		payloadAvailability: newPayloadAvailability(),
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		v.waitForPayloadAvailableOrDeadline(ctx, 0)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not return after context cancellation")
	}
}
