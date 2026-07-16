//go:build !bazel

package bazel

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/build/externaldata"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestFetchOnMissUnknownPath(t *testing.T) {
	require.Equal(t, false, fetchOnMiss("some/other/path.txt"))
}

func TestArchiveForPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantName string
		wantOK   bool
	}{
		{"consensus general", "tests/general/phase0/foo.ssz", externaldata.ConsensusSpecTestsGeneral, true},
		{"consensus minimal", "tests/minimal/altair/bar.yaml", externaldata.ConsensusSpecTestsMinimal, true},
		{"consensus mainnet", "tests/mainnet/capella/baz.yaml", externaldata.ConsensusSpecTestsMainnet, true},
		{"consensus unknown category", "tests/devnet/foo.yaml", "", false},
		{"consensus bare", "tests", "", false},
		{"external repo", "external/my_repo/file.txt", "my_repo", true},
		{"external nested repo", "external/my_repo/deep/file.txt", "my_repo", true},
		{"external empty repo", "external/", "", false},
		{"external bare", "external", "", false},
		{"bls category", "aggregate/small/foo.yaml", externaldata.BLSSpecTests, true},
		{"bls verify category", "verify/case/foo.yaml", externaldata.BLSSpecTests, true},
		{"unrelated path", "some/other/path.txt", "", false},
		{"empty path", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotOK := archiveForPath(tt.path)
			require.Equal(t, tt.wantName, gotName)
			require.Equal(t, tt.wantOK, gotOK)
		})
	}
}

func TestBLSCategoriesAreArchivePaths(t *testing.T) {
	for category := range blsCategories {
		name, ok := archiveForPath(category + "/case/foo.yaml")
		require.Equal(t, true, ok, "BLS category %q did not resolve to an archive", category)
		require.Equal(t, externaldata.BLSSpecTests, name, "BLS category %q resolved to the wrong archive", category)
	}
}
