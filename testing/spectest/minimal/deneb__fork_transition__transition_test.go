package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/deneb/fork"
)

func TestMinimal_Deneb_Transition(t *testing.T) {
	fork.RunForkTransitionTest(t, "minimal")
}
