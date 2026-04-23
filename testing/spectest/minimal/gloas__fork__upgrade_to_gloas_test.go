package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/fork"
)

func TestMinimal_UpgradeToGloas(t *testing.T) {
	t.Skip("gloas spec tests disabled until https://github.com/OffchainLabs/prysm/pull/16658")
	fork.RunUpgradeToGloas(t, "minimal")
}
