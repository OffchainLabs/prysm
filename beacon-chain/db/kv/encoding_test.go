package kv

import (
	"testing"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	testpb "github.com/OffchainLabs/prysm/v7/proto/testing"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func Test_encode_handlesNilFromFunction(t *testing.T) {
	foo := func() *testpb.Puzzle {
		return nil
	}
	_, err := encode(t.Context(), foo())
	require.ErrorContains(t, "cannot encode nil message", err)
}

func TestIsSSZStorageFormat_AttestationGloas(t *testing.T) {
	require.Equal(t, true, isSSZStorageFormat(&ethpb.AttestationGloas{}))
}
