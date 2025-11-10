package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/altair/fork"
)

func TestMinimal_Altair_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "minimal")
}
