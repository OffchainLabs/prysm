package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/fork"
)

func TestMinimal_Gloas_Transition(t *testing.T) {
	t.Skip("gloas spec tests disabled until https://github.com/OffchainLabs/prysm/pull/16658")
	fork.RunForkTransitionTest(t, "minimal")
}
