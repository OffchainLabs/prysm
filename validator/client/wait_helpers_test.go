package client

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
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
	component := cfg.AttestationDueBPS

	got, err := v.slotComponentDeadline(slot, component)
	require.NoError(t, err)

	startTime, err := slots.StartTime(v.genesisTime, slot)
	require.NoError(t, err)
	expected := startTime.Add(cfg.SlotComponentDuration(component))

	require.Equal(t, expected, got)
}

func TestSlotComponentSpanName(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	cfg := params.BeaconConfig()
	v := &validator{}
	tests := []struct {
		name      string
		component primitives.BP
		expected  string
	}{
		{
			name:      "attestation",
			component: cfg.AttestationDueBPS,
			expected:  "validator.waitAttestationWindow",
		},
		{
			name:      "aggregate",
			component: cfg.AggregateDueBPS,
			expected:  "validator.waitAggregateWindow",
		},
		{
			name:      "default",
			component: cfg.AttestationDueBPS + 7,
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
		v.waitUntilSlotComponent(ctx, 1, cfg.AttestationDueBPS)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitUntilSlotComponent did not return after context cancellation")
	}
}

func TestWaitUntilSlotOffset_ReturnsImmediatelyWhenOffsetElapsed(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	// Genesis far enough in the past that slot 1's start + offset is already elapsed.
	v := &validator{genesisTime: time.Now().Add(-time.Hour)}

	done := make(chan struct{})
	go func() {
		v.waitUntilSlotOffset(context.Background(), 1, 1500*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitUntilSlotOffset did not return when the offset had already elapsed")
	}
}

func TestWaitUntilSlotOffset_ContextCancelReturnsImmediately(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	// Genesis in the future so the offset has not elapsed; only ctx cancel unblocks.
	v := &validator{genesisTime: time.Now().Add(time.Hour)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		v.waitUntilSlotOffset(ctx, 1, 2*time.Second)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitUntilSlotOffset did not return after context cancellation")
	}
}

func TestProposalReleaseDelay(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	// Pin the mainnet config: the bounds below assume the pre-Gloas attestation
	// deadline, and other tests in this package activate Gloas globally.
	params.OverrideBeaconConfig(params.MainnetConfig().Copy())
	cfg := params.BeaconConfig()

	v := &validator{genesisTime: time.Now()}
	slot := primitives.Slot(5)

	// Compute the fork-appropriate bounds the same way the implementation does.
	dueBPS := cfg.AttestationDueBPS
	if slots.ToEpoch(slot) >= cfg.GloasForkEpoch {
		dueBPS = cfg.AttestationDueBPSGloas
	}
	maxDelay := cfg.SlotComponentDuration(dueBPS) - proposerTimingGameSafetyBudget
	reorgCutoff := cfg.SlotComponentDuration(cfg.ProposerReorgCutoffBPS)
	require.Equal(t, true, maxDelay > 0)
	require.Equal(t, true, reorgCutoff < maxDelay)

	tests := []struct {
		name       string
		configured time.Duration
		want       time.Duration
	}{
		{name: "disabled returns zero", configured: 0, want: 0},
		{name: "below cutoff passes through", configured: reorgCutoff - time.Second, want: reorgCutoff - time.Second},
		{name: "beyond reorg cutoff still allowed", configured: reorgCutoff + 100*time.Millisecond, want: reorgCutoff + 100*time.Millisecond},
		{name: "above max is clamped", configured: maxDelay + time.Second, want: maxDelay},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{ProposerTimingGameDelay: tt.configured})
			defer reset()
			assert.Equal(t, tt.want, v.proposalReleaseDelay(slot))
		})
	}
}
