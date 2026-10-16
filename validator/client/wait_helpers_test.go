package client

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestSlotComponentDeadline(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	cfg := params.BeaconConfig()
	v := &validator{genesisTime: time.Unix(1700000000, 0)}
	slot := primitives.Slot(5)

	got, err := v.slotComponentDeadline(slot, params.AttestationDue)
	require.NoError(t, err)

	startTime, err := slots.StartTime(v.genesisTime, slot)
	require.NoError(t, err)
	expected := startTime.Add(cfg.SlotComponentDuration(cfg.AttestationDueBPS))

	require.Equal(t, expected, got)
}

func TestBeforeSlotComponent(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	component := params.PayloadAttestationDue

	t.Run("deadline still ahead", func(t *testing.T) {
		v := &validator{genesisTime: time.Now()}
		require.Equal(t, true, v.beforeSlotComponent(1, component))
	})

	t.Run("deadline already elapsed", func(t *testing.T) {
		v := &validator{genesisTime: time.Time{}}
		require.Equal(t, false, v.beforeSlotComponent(1, component))
	})

	t.Run("unreachable deadline reports false", func(t *testing.T) {
		v := &validator{genesisTime: time.Now()}
		require.Equal(t, false, v.beforeSlotComponent(primitives.Slot(math.MaxUint64), component))
	})
}

func TestSlotComponentSpanName(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	v := &validator{}
	tests := []struct {
		name      string
		component params.SlotComponent
		expected  string
	}{
		{
			name:      "attestation",
			component: params.AttestationDue,
			expected:  "validator.waitAttestationWindow",
		},
		{
			name:      "aggregate",
			component: params.AggregateDue,
			expected:  "validator.waitAggregateWindow",
		},
		{
			name:      "default",
			component: params.SlotComponent(200),
			expected:  "validator.waitSlotComponent",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, v.slotComponentSpanName(tt.component))
		})
	}
}

func TestWaitUntilSlotComponent_ContextCancelReturnsImmediately(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotDurationMilliseconds = 10000
	params.OverrideBeaconConfig(cfg)

	v := &validator{genesisTime: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		v.waitUntilSlotComponent(ctx, 1, params.AttestationDue)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitUntilSlotComponent did not return after context cancellation")
	}
}
