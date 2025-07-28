package stateutil_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestProposerLookaheadRoot(t *testing.T) {
	lookahead := make([]primitives.ValidatorIndex, 64)
	root, err := stateutil.ProposerLookaheadRoot(lookahead)
	require.NoError(t, err)
	expected := [32]byte{83, 109, 152, 131, 127, 45, 209, 101, 165, 93, 94, 234, 233, 20, 133, 149, 68, 114, 213, 111, 36, 109, 242, 86, 191, 60, 174, 25, 53, 42, 18, 60}
	require.Equal(t, expected, root)
}
