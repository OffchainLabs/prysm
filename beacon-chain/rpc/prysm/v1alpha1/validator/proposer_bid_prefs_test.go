//go:build minimal

package validator

import (
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestBuilderBidGate exercises the per-entry floor and boost gate each builder
// bid must clear against the local self-build before ranking.
func TestBuilderBidGate(t *testing.T) {
	tests := []struct {
		name  string
		eff   primitives.Gwei
		local primitives.Gwei
		prefs bidPreferences
		want  bool
	}{
		{name: "neutral 100 higher wins", eff: 1000, local: 100, prefs: bidPreferences{boostFactor: 100}, want: true},
		{name: "neutral 100 lower loses", eff: 100, local: 1000, prefs: bidPreferences{boostFactor: 100}, want: false},
		{name: "boost 0 never wins", eff: 1000, local: 0, prefs: bidPreferences{boostFactor: 0}, want: false},
		{name: "boost max always wins", eff: 1, local: 1000, prefs: bidPreferences{boostFactor: math.MaxUint64}, want: true},
		{name: "boost 200 lets 60 beat 100", eff: 60, local: 100, prefs: bidPreferences{boostFactor: 200}, want: true},
		{name: "below min_bid rejected", eff: 99, local: 0, prefs: bidPreferences{boostFactor: 100, minBid: 100}, want: false},
		{name: "at min_bid accepted", eff: 100, local: 0, prefs: bidPreferences{boostFactor: 100, minBid: 100}, want: true},
		{name: "min_bid gates max boost", eff: 99, local: 0, prefs: bidPreferences{boostFactor: math.MaxUint64, minBid: 100}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, builderBidQualifies(tt.eff, tt.local, tt.prefs))
		})
	}
}

func TestBuilderBeatsLocal(t *testing.T) {
	tests := []struct {
		name         string
		builderValue primitives.Gwei
		localValue   primitives.Gwei
		boostFactor  uint64
		want         bool
	}{
		{"neutral higher wins", 200, 100, 100, true},
		{"neutral equal loses", 100, 100, 100, false},
		{"neutral lower loses", 50, 100, 100, false},
		{"boost 0 always local", 1000, 0, 0, false},
		{"boost max always builder", 1, 1000, math.MaxUint64, true},
		{"boost 200 lifts 60 over 100", 60, 100, 200, true},
		{"boost 200 leaves 40 under 100", 40, 100, 200, false},
		// Large values must not overflow uint64 math (bestBid uses big.Int internally).
		{"no overflow on large values", primitives.Gwei(1) << 40, primitives.Gwei(1) << 40, 200, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, builderBeatsLocal(tt.builderValue, tt.localValue, tt.boostFactor))
		})
	}
}

// A bid whose value+payment would wrap uint64 must saturate, never rank below honest bids.
func TestEffectiveBidValue_Saturates(t *testing.T) {
	const builderIdx = primitives.BuilderIndex(3)
	overflow := newBid(primitives.Gwei(math.MaxUint64-5), 100, builderIdx)
	require.Equal(t, primitives.Gwei(math.MaxUint64), effectiveBidValue(overflow, math.MaxUint64))

	honest := newBid(50, 25, builderIdx)
	require.Equal(t, primitives.Gwei(75), effectiveBidValue(honest, math.MaxUint64))
	require.Equal(t, primitives.Gwei(60), effectiveBidValue(honest, 10))
}
