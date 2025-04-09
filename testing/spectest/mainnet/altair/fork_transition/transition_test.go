package fork_transition

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/altair/fork"
)

func TestMainnet_Altair_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "mainnet")
}
