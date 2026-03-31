package stateutil_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestPTCWindowRoot_NilEntry(t *testing.T) {
	_, err := stateutil.PTCWindowRoot([]*ethpb.PTCs{{ValidatorIndices: make([]primitives.ValidatorIndex, fieldparams.PTCSize)}, nil})
	require.ErrorContains(t, "invalid PTC at position 1", err)
}
