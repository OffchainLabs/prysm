package peerdas_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestExtendedSampleCount(t *testing.T) {
	const samplesPerSlot = 16

	testCases := []struct {
		name                string
		allowedMissings     uint64
		extendedSampleCount uint64
	}{
		{name: "allowedMissings=0", allowedMissings: 0, extendedSampleCount: 16},
		{name: "allowedMissings=1", allowedMissings: 1, extendedSampleCount: 20},
		{name: "allowedMissings=2", allowedMissings: 2, extendedSampleCount: 24},
		{name: "allowedMissings=3", allowedMissings: 3, extendedSampleCount: 27},
		{name: "allowedMissings=4", allowedMissings: 4, extendedSampleCount: 29},
		{name: "allowedMissings=5", allowedMissings: 5, extendedSampleCount: 32},
		{name: "allowedMissings=6", allowedMissings: 6, extendedSampleCount: 35},
		{name: "allowedMissings=7", allowedMissings: 7, extendedSampleCount: 37},
		{name: "allowedMissings=8", allowedMissings: 8, extendedSampleCount: 40},
		{name: "allowedMissings=9", allowedMissings: 9, extendedSampleCount: 42},
		{name: "allowedMissings=10", allowedMissings: 10, extendedSampleCount: 44},
		{name: "allowedMissings=11", allowedMissings: 11, extendedSampleCount: 47},
		{name: "allowedMissings=12", allowedMissings: 12, extendedSampleCount: 49},
		{name: "allowedMissings=13", allowedMissings: 13, extendedSampleCount: 51},
		{name: "allowedMissings=14", allowedMissings: 14, extendedSampleCount: 53},
		{name: "allowedMissings=15", allowedMissings: 15, extendedSampleCount: 55},
		{name: "allowedMissings=16", allowedMissings: 16, extendedSampleCount: 57},
		{name: "allowedMissings=17", allowedMissings: 17, extendedSampleCount: 59},
		{name: "allowedMissings=18", allowedMissings: 18, extendedSampleCount: 61},
		{name: "allowedMissings=19", allowedMissings: 19, extendedSampleCount: 63},
		{name: "allowedMissings=20", allowedMissings: 20, extendedSampleCount: 65},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := peerdas.ExtendedSampleCount(samplesPerSlot, tc.allowedMissings)
			require.Equal(t, tc.extendedSampleCount, result)
		})
	}
}

func TestHypergeomCDF(t *testing.T) {
	// Test case from https://en.wikipedia.org/wiki/Hypergeometric_distribution
	// Population size: 1000, number of successes in population: 500, sample size: 10, number of successes in sample: 5
	// Expected result: 0.072
	const (
		expected = 0.0796665913283742
		margin   = 0.000001
	)

	actual := peerdas.HypergeomCDF(5, 128, 65, 16)
	require.Equal(t, true, expected-margin <= actual && actual <= expected+margin)
}
