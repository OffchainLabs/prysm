package client

import (
	"context"
	"sync/atomic"
	"testing"

	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"go.uber.org/mock/gomock"
)

func TestValidator_connTracker(t *testing.T) {
	t.Run("change persists until the push is confirmed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := validatormock.NewMockValidatorClient(ctrl)
		var connGen atomic.Uint64
		client.EXPECT().ConnectionGeneration().DoAndReturn(connGen.Load).AnyTimes()
		v := &validator{validatorClient: client}

		// Counter starts at 0, matching the zero confirmed generation.
		require.Equal(t, false, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))

		// A fallback switch bumps the counter — detected, and stays detected
		// until a push is confirmed (e.g. the first submission failed).
		connGen.Store(1)
		gen := v.connGeneration()
		require.Equal(t, true, v.connTracker.changed(proposerPrefsPush, gen))
		require.Equal(t, true, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))

		v.connTracker.confirm(proposerPrefsPush, gen)
		require.Equal(t, false, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))

		// A round-robin bounce (host0 → host1 → host0) leaves the host unchanged
		// but advances the counter twice; still detected.
		connGen.Store(3)
		gen = v.connGeneration()
		require.Equal(t, true, v.connTracker.changed(proposerPrefsPush, gen))
		// A stale confirmation from an in-flight push started before the switch
		// must not mask the newer generation.
		v.connTracker.confirm(proposerPrefsPush, 1)
		require.Equal(t, true, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))
		v.connTracker.confirm(proposerPrefsPush, gen)
		require.Equal(t, false, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))
	})
}

// EnsureEventStream must start when the stream is down, no-op while it runs on
// the current host, and replace it after a fallback host switch.
func TestValidator_EnsureEventStream_RebindsOnHostSwitch(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := validatormock.NewMockValidatorClient(ctrl)
	var connGen atomic.Uint64
	client.EXPECT().ConnectionGeneration().DoAndReturn(connGen.Load).AnyTimes()
	v := &validator{validatorClient: client}
	topics := []string{"head"}

	// The client stream is started in a goroutine; wait for it before moving on.
	started := make(chan struct{}, 2)
	notifyStarted := func(context.Context, []string, chan<- *eventClient.Event) {
		started <- struct{}{}
	}

	// Not running: starts and binds the current generation.
	client.EXPECT().EventStreamIsRunning().Return(false)
	client.EXPECT().StartEventStream(gomock.Any(), gomock.Any(), gomock.Any()).Do(notifyStarted)
	v.EnsureEventStream(t.Context(), topics)
	<-started

	// Running on an unchanged host: no-op.
	client.EXPECT().EventStreamIsRunning().Return(true)
	v.EnsureEventStream(t.Context(), topics)

	// Running but the host switched: replaced.
	connGen.Store(1)
	client.EXPECT().EventStreamIsRunning().Return(true)
	client.EXPECT().StartEventStream(gomock.Any(), gomock.Any(), gomock.Any()).Do(notifyStarted)
	v.EnsureEventStream(t.Context(), topics)
	<-started
}
