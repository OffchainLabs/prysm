package version

import (
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionString(t *testing.T) {
	tests := []struct {
		name    string
		version int
		want    string
	}{
		{
			name:    "phase0",
			version: Phase0,
			want:    "phase0",
		},
		{
			name:    "altair",
			version: Altair,
			want:    "altair",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := String(tt.version); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersionSorting(t *testing.T) {
	versions := All()
	expected := slices.Clone(versions)
	sort.Ints(expected)
	tests := []struct {
		name     string
		expected []int
	}{
		{
			name:     "allVersions sorted in ascending order",
			expected: expected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, versions, "allVersions should match expected order")
		})
	}
}
