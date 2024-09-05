package blockchain

import (
	"testing"

	doublylinkedtree "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestServiceGetPTCVote(t *testing.T) {
	c := &currentlySyncingPayload{roots: make(map[[32]byte]primitives.PTCStatus)}
	s := &Service{cfg: &config{ForkChoiceStore: doublylinkedtree.New()}, payloadBeingSynced: c}
	r := [32]byte{'r'}
	require.Equal(t, primitives.PAYLOAD_ABSENT, s.GetPTCVote(r))
	c.roots[r] = primitives.PAYLOAD_WITHHELD
	require.Equal(t, primitives.PAYLOAD_WITHHELD, s.GetPTCVote(r))
}
