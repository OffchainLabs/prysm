package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/deneb/fork"
)

func TestMinimal_UpgradeToDeneb(t *testing.T) {
	fork.RunUpgradeToDeneb(t, "minimal")
}
