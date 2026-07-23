// Package testing provides shared test helpers for the consensus-types
// block interfaces.
package testing

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// RequireBlocksEqual asserts that two signed beacon blocks are equal by their
// SSZ-logical proto content. Unlike reflect.DeepEqual on the block wrappers, this
// treats nil and empty slices as equal, which matters because an empty list
// round-trips through SSZ (e.g. storage) as a non-nil empty slice.
func RequireBlocksEqual(t *testing.T, want, got interfaces.ReadOnlySignedBeaconBlock) {
	t.Helper()
	wp, err := want.Proto()
	require.NoError(t, err)
	gp, err := got.Proto()
	require.NoError(t, err)
	require.DeepSSZEqual(t, wp, gp)
}
