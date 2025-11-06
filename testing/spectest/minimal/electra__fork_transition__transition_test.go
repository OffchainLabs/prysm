package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/electra/fork"
)

func TestMinimal_Electra_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "minimal")
}
