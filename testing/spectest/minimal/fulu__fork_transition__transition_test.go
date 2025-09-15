package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/fork"
)

func TestMinimal_Fulu_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "minimal")
}
