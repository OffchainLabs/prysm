//go:build minimal

package validator

import (
	"math"
	"testing"

	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestBestBid_BoostFactor exercises the boost-factor and min-bid preferences that
// gate whether a builder bid can beat the local self-build.
func TestBestBid_BoostFactor(t *testing.T) {
	const builderIdx = primitives.BuilderIndex(2)
	tests := []struct {
		name    string
		local   *consensusblocks.GetPayloadResponse
		builder *ethpb.SignedExecutionPayloadBid
		prefs   bidPreferences
		wantSrc bidSource
		wantNil bool
	}{
		{
			name:    "neutral 100 higher builder wins",
			local:   localWithGwei(100),
			builder: newBid(1000, 0, builderIdx),
			prefs:   bidPreferences{maxPayment: 1000, boostFactor: 100},
			wantSrc: bidSourceBuilderAPI,
		},
		{
			name:    "neutral 100 lower builder loses",
			local:   localWithGwei(1000),
			builder: newBid(100, 0, builderIdx),
			prefs:   bidPreferences{maxPayment: 1000, boostFactor: 100},
			wantSrc: bidSourceSelfBuild,
			wantNil: true,
		},
		{
			name:    "boost 0 prefers local even when builder is higher",
			local:   localWithGwei(0),
			builder: newBid(1000, 0, builderIdx),
			prefs:   bidPreferences{maxPayment: 1000, boostFactor: 0},
			wantSrc: bidSourceSelfBuild,
			wantNil: true,
		},
		{
			name:    "boost max prefers builder even when local is higher",
			local:   localWithGwei(1000),
			builder: newBid(1, 0, builderIdx),
			prefs:   bidPreferences{maxPayment: 1000, boostFactor: math.MaxUint64},
			wantSrc: bidSourceBuilderAPI,
		},
		{
			name:    "boost 200 lets 60 beat local 100",
			local:   localWithGwei(100),
			builder: newBid(60, 0, builderIdx),
			prefs:   bidPreferences{maxPayment: 1000, boostFactor: 200},
			wantSrc: bidSourceBuilderAPI,
		},
		{
			name:    "min_bid rejects builder below floor",
			local:   localWithGwei(0),
			builder: newBid(50, 0, builderIdx),
			prefs:   bidPreferences{maxPayment: 1000, boostFactor: 100, minBid: 100},
			wantSrc: bidSourceSelfBuild,
			wantNil: true,
		},
		{
			name:    "min_bid allows builder at floor",
			local:   localWithGwei(0),
			builder: newBid(100, 0, builderIdx),
			prefs:   bidPreferences{maxPayment: 1000, boostFactor: 100, minBid: 100},
			wantSrc: bidSourceBuilderAPI,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, src := bestBid(tt.local, nil, tt.builder, tt.prefs)
			require.Equal(t, tt.wantSrc, src)
			if tt.wantNil {
				require.IsNil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, builderIdx, got.Message.BuilderIndex)
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
