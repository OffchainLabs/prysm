package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/fork"
)

func TestMinimal_Gloas_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "minimal")
}
