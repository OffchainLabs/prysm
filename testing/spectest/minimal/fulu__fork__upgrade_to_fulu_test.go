package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/fulu/fork"
)

func TestMinimal_UpgradeToFulu(t *testing.T) {
	fork.RunUpgradeToFulu(t, "minimal")
}
