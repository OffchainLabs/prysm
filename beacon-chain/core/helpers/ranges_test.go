package helpers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestSortedSliceFromMap(t *testing.T) {
	input := map[uint64]bool{5: true, 3: true, 8: true, 1: true}
	expected := []uint64{1, 3, 5, 8}

	actual := helpers.SortedSliceFromMap(input)
	require.Equal(t, len(expected), len(actual))

	for i := range expected {
		require.Equal(t, expected[i], actual[i])
	}
}

func TestPrettySlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []uint64
		expected string
	}{
		{
			name:     "empty slice",
			input:    []uint64{},
			expected: "",
		},
		{
			name:     "only distinct elements",
			input:    []uint64{1, 3, 5, 7, 9},
			expected: "1,3,5,7,9",
		},
		{
			name:     "single range",
			input:    []uint64{1, 2, 3, 4, 5},
			expected: "1-5",
		},
		{
			name:     "multiple ranges and distinct elements",
			input:    []uint64{1, 2, 3, 5, 6, 7, 8, 10, 12, 13, 14},
			expected: "1-3,5-8,10,12-14",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := helpers.PrettySlice(tt.input)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestSortedPrettySliceFromMap(t *testing.T) {
	input := map[uint64]bool{5: true, 7: true, 8: true, 10: true}
	expected := "5,7-8,10"

	actual := helpers.SortedPrettySliceFromMap(input)
	require.Equal(t, expected, actual)
}
