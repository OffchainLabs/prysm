package version_test

import (
	"slices"
	"sort"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionString(t *testing.T) {
	tests := []struct {
		name    string
		version int
		want    string
	}{
		{
			name:    "phase0",
			version: version.Phase0,
			want:    "phase0",
		},
		{
			name:    "altair",
			version: version.Altair,
			want:    "altair",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := version.String(tt.version); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersionSorting(t *testing.T) {
	versions := version.All()
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

func TestBefore(t *testing.T) {
	tests := []struct {
		name   string
		cutoff int
		want   []int
	}{
		{
			name:   "before first fork",
			cutoff: version.Phase0 - 1,
		},
		{
			name:   "at first fork",
			cutoff: version.Phase0,
		},
		{
			name:   "first fork only",
			cutoff: version.Altair,
			want:   []int{version.Phase0},
		},
		{
			name:   "middle fork is excluded",
			cutoff: version.Deneb,
			want:   []int{version.Phase0, version.Altair, version.Bellatrix, version.Capella},
		},
		{
			name:   "before gloas",
			cutoff: version.Gloas,
			want:   []int{version.Phase0, version.Altair, version.Bellatrix, version.Capella, version.Deneb, version.Electra, version.Fulu},
		},
		{
			name:   "after latest fork returns all supported forks",
			cutoff: version.Gloas + 1,
			want:   slices.Clone(version.All()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := version.Before(tt.cutoff)
			if !slices.Equal(tt.want, got) {
				t.Errorf("Before(%d) = %v, want %v", tt.cutoff, got, tt.want)
			}
		})
	}
}

func TestBefore_AppendDoesNotChangeAll(t *testing.T) {
	all := version.All()
	original := slices.Clone(all)
	t.Cleanup(func() {
		copy(all, original)
	})

	got := append(version.Before(version.Altair), version.Gloas)
	assert.Equal(t, []int{version.Phase0, version.Gloas}, got)
	assert.Equal(t, original, version.All(), "appending to Before must not overwrite supported fork versions")
}

func TestUnsupportedVersionsExcludedFromAll(t *testing.T) {
	for _, v := range unsupportedVersions() {
		assert.NotContains(t, version.All(), v, "unsupported version %s should not be returned by version.All()", version.String(v))
	}
}

func TestUnsupportedVersionsAreNotScheduledOnTestnets(t *testing.T) {
	unsupported := unsupportedVersions()
	if len(unsupported) == 0 {
		t.Skip("no unsupported versions defined")
	}

	testnetConfigs := []*params.BeaconChainConfig{
		params.HoleskyConfig(),
		params.SepoliaConfig(),
		params.HoodiConfig(),
	}

	for _, v := range unsupported {
		for _, cfg := range testnetConfigs {
			epoch := forkEpochForVersion(cfg, v)
			require.Equalf(
				t,
				cfg.FarFutureEpoch,
				epoch,
				"unsupported version %s should not be scheduled on %s (epoch=%d)",
				version.String(v),
				cfg.ConfigName,
				epoch,
			)
		}
	}
}

func forkEpochForVersion(cfg *params.BeaconChainConfig, v int) primitives.Epoch {
	switch v {
	case version.Phase0:
		return cfg.GenesisEpoch
	case version.Altair:
		return cfg.AltairForkEpoch
	case version.Bellatrix:
		return cfg.BellatrixForkEpoch
	case version.Capella:
		return cfg.CapellaForkEpoch
	case version.Deneb:
		return cfg.DenebForkEpoch
	case version.Electra:
		return cfg.ElectraForkEpoch
	case version.Fulu:
		return cfg.FuluForkEpoch
	default:
		if version.IsUnsupported(v) {
			return cfg.FarFutureEpoch
		}
		panic("forkEpochForVersion missing version " + version.String(v))
	}
}

func unsupportedVersions() []int {
	var unsupportedVersions []int
	for v := 0; ; v++ {
		if version.String(v) == "unknown version" {
			break
		}
		if version.IsUnsupported(v) {
			unsupportedVersions = append(unsupportedVersions, v)
		}
	}
	return unsupportedVersions
}
