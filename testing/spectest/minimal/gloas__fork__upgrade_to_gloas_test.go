package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/fork"
)

func TestMinimal_UpgradeToGloas(t *testing.T) {
	fork.RunUpgradeToGloas(t, "minimal")
}
