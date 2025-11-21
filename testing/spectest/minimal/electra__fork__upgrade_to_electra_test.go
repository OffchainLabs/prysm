package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/electra/fork"
)

func TestMinimal_UpgradeToElectra(t *testing.T) {
	fork.RunUpgradeToElectra(t, "minimal")
}
